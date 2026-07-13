package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	wsOpContinuation = 0x0
	wsOpText         = 0x1
	wsOpBinary       = 0x2
	wsOpClose        = 0x8
	wsOpPing         = 0x9
	wsOpPong         = 0xA

	maxWebSocketMessage = 16 * 1024 * 1024
)

type WSConn struct {
	conn    net.Conn
	reader  *bufio.Reader
	writeMu sync.Mutex
}

func DialWebSocket(ctx context.Context, endpoint, token string, insecureSkipVerify bool) (*WSConn, error) {
	u, err := buildRPCURL(endpoint, token)
	if err != nil {
		return nil, err
	}

	dialer := &net.Dialer{Timeout: 10 * time.Second}
	raw, err := dialer.DialContext(ctx, "tcp", dialAddress(u))
	if err != nil {
		return nil, err
	}
	conn := raw
	if u.Scheme == "wss" {
		tlsConn := tls.Client(raw, &tls.Config{
			ServerName:         u.Hostname(),
			InsecureSkipVerify: insecureSkipVerify,
			MinVersion:         tls.VersionTLS12,
		})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			_ = raw.Close()
			return nil, err
		}
		conn = tlsConn
	}

	key, err := websocketKey()
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	requestURI := u.RequestURI()
	if requestURI == "" {
		requestURI = "/"
	}
	if err := conn.SetDeadline(time.Now().Add(15 * time.Second)); err != nil {
		_ = conn.Close()
		return nil, err
	}
	_, err = fmt.Fprintf(conn,
		"GET %s HTTP/1.1\r\nHost: %s\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Key: %s\r\nSec-WebSocket-Version: 13\r\nUser-Agent: fake-komari-agent/%s\r\n\r\n",
		requestURI, u.Host, key, appVersion,
	)
	if err != nil {
		_ = conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	resp, err := http.ReadResponse(br, &http.Request{Method: http.MethodGet})
	if err != nil {
		_ = conn.Close()
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		_ = conn.Close()
		return nil, fmt.Errorf("websocket upgrade failed: %s", resp.Status)
	}
	if !strings.EqualFold(resp.Header.Get("Upgrade"), "websocket") {
		_ = conn.Close()
		return nil, errors.New("websocket upgrade failed: missing Upgrade header")
	}
	if !headerContainsToken(resp.Header.Get("Connection"), "upgrade") {
		_ = conn.Close()
		return nil, errors.New("websocket upgrade failed: missing Connection upgrade")
	}
	if got, want := resp.Header.Get("Sec-WebSocket-Accept"), websocketAccept(key); got != want {
		_ = conn.Close()
		return nil, errors.New("websocket upgrade failed: invalid accept key")
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return &WSConn{conn: conn, reader: br}, nil
}

func buildRPCURL(endpoint, token string) (*url.URL, error) {
	u, err := parseEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	case "ws", "wss":
	default:
		return nil, fmt.Errorf("unsupported endpoint scheme %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("endpoint host is empty")
	}

	path := strings.TrimRight(u.Path, "/")
	if !strings.HasSuffix(path, "/api/clients/v2/rpc") {
		path = path + "/api/clients/v2/rpc"
	}
	if path == "" {
		path = "/api/clients/v2/rpc"
	}
	u.Path = path
	q := u.Query()
	q.Set("token", token)
	u.RawQuery = q.Encode()
	return u, nil
}

func dialAddress(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "wss" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return net.JoinHostPort(u.Hostname(), port)
}

func websocketKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b[:]), nil
}

func websocketAccept(key string) string {
	const guid = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"
	sum := sha1.Sum([]byte(key + guid))
	return base64.StdEncoding.EncodeToString(sum[:])
}

func headerContainsToken(value, token string) bool {
	for _, part := range strings.Split(value, ",") {
		if strings.EqualFold(strings.TrimSpace(part), token) {
			return true
		}
	}
	return false
}

func (c *WSConn) SetReadDeadline(t time.Time) error {
	return c.conn.SetReadDeadline(t)
}

func (c *WSConn) WriteJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return c.writeFrame(wsOpText, b)
}

func (c *WSConn) WriteControl(opcode byte, payload []byte) error {
	if len(payload) > 125 {
		return errors.New("websocket control payload too large")
	}
	return c.writeFrame(opcode, payload)
}

func (c *WSConn) ReadMessage() (byte, []byte, error) {
	var message []byte
	var messageOpcode byte
	for {
		opcode, payload, fin, err := c.readFrame()
		if err != nil {
			return 0, nil, err
		}
		switch opcode {
		case wsOpText, wsOpBinary:
			if len(payload) > maxWebSocketMessage {
				return 0, nil, errors.New("websocket message too large")
			}
			if fin {
				return opcode, payload, nil
			}
			messageOpcode = opcode
			message = append(message[:0], payload...)
		case wsOpContinuation:
			if messageOpcode == 0 {
				return 0, nil, errors.New("unexpected websocket continuation frame")
			}
			message = append(message, payload...)
			if len(message) > maxWebSocketMessage {
				return 0, nil, errors.New("websocket message too large")
			}
			if fin {
				return messageOpcode, message, nil
			}
		case wsOpPing:
			if err := c.WriteControl(wsOpPong, payload); err != nil {
				return 0, nil, err
			}
		case wsOpPong:
			continue
		case wsOpClose:
			_ = c.WriteControl(wsOpClose, payload)
			return wsOpClose, payload, io.EOF
		default:
			return 0, nil, fmt.Errorf("unsupported websocket opcode %d", opcode)
		}
	}
}

func (c *WSConn) readFrame() (byte, []byte, bool, error) {
	var header [2]byte
	if _, err := io.ReadFull(c.reader, header[:]); err != nil {
		return 0, nil, false, err
	}
	fin := header[0]&0x80 != 0
	if header[0]&0x70 != 0 {
		return 0, nil, false, errors.New("websocket frame uses unsupported extensions")
	}
	opcode := header[0] & 0x0F
	masked := header[1]&0x80 != 0
	length := uint64(header[1] & 0x7F)
	switch length {
	case 126:
		var ext [2]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return 0, nil, false, err
		}
		length = uint64(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err := io.ReadFull(c.reader, ext[:]); err != nil {
			return 0, nil, false, err
		}
		length = binary.BigEndian.Uint64(ext[:])
	}
	if length > maxWebSocketMessage {
		return 0, nil, false, errors.New("websocket frame too large")
	}

	var mask [4]byte
	if masked {
		if _, err := io.ReadFull(c.reader, mask[:]); err != nil {
			return 0, nil, false, err
		}
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(c.reader, payload); err != nil {
		return 0, nil, false, err
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i%4]
		}
	}
	return opcode, payload, fin, nil
}

func (c *WSConn) writeFrame(opcode byte, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	header := make([]byte, 0, 14)
	header = append(header, 0x80|opcode)
	length := len(payload)
	switch {
	case length <= 125:
		header = append(header, 0x80|byte(length))
	case length <= 65535:
		header = append(header, 0x80|126)
		var ext [2]byte
		binary.BigEndian.PutUint16(ext[:], uint16(length))
		header = append(header, ext[:]...)
	default:
		header = append(header, 0x80|127)
		var ext [8]byte
		binary.BigEndian.PutUint64(ext[:], uint64(length))
		header = append(header, ext[:]...)
	}
	header = append(header, mask[:]...)
	frame := make([]byte, len(header)+len(payload))
	copy(frame, header)
	copy(frame[len(header):], payload)
	for i := range payload {
		frame[len(header)+i] ^= mask[i%4]
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second)); err != nil {
		return err
	}
	_, err := c.conn.Write(frame)
	return err
}

func (c *WSConn) Close() error {
	_ = c.WriteControl(wsOpClose, []byte{0x03, 0xE8})
	return c.conn.Close()
}
