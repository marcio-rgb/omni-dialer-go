package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

/**
 * Cliente WebSocket RFC 6455 nativo sem dependências externas.
 *
 * @pattern Adapter
 * @governedBy .agents/ARCHITECT.md
 */

type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
}

func connectWebSocket(rawURL string, timeout time.Duration) (*wsConn, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	host := u.Host
	if !strings.Contains(host, ":") {
		host += ":80"
	}

	conn, err := net.DialTimeout("tcp", host, timeout)
	if err != nil {
		return nil, err
	}

	req := fmt.Sprintf("GET %s HTTP/1.1\r\n"+
		"Host: %s\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==\r\n"+
		"Sec-WebSocket-Version: 13\r\n\r\n", u.RequestURI(), u.Host)

	_ = conn.SetDeadline(time.Now().Add(timeout))
	if _, err := conn.Write([]byte(req)); err != nil {
		conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			conn.Close()
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	_ = conn.SetDeadline(time.Time{})
	return &wsConn{conn: conn, br: br}, nil
}

func (w *wsConn) WriteText(text string) error {
	return w.writeFrame(0x01, []byte(text))
}

func (w *wsConn) WriteBinary(data []byte) error {
	return w.writeFrame(0x02, data)
}

func (w *wsConn) writeFrame(opcode byte, payload []byte) error {
	var header bytes.Buffer
	header.WriteByte(0x80 | opcode) // FIN + opcode

	length := len(payload)
	maskBit := byte(0x80)

	if length <= 125 {
		header.WriteByte(maskBit | byte(length))
	} else if length <= 65535 {
		header.WriteByte(maskBit | 126)
		_ = binary.Write(&header, binary.BigEndian, uint16(length))
	} else {
		header.WriteByte(maskBit | 127)
		_ = binary.Write(&header, binary.BigEndian, uint64(length))
	}

	var mask [4]byte
	_, _ = rand.Read(mask[:])
	header.Write(mask[:])

	masked := make([]byte, length)
	for i := 0; i < length; i++ {
		masked[i] = payload[i] ^ mask[i%4]
	}

	if _, err := w.conn.Write(header.Bytes()); err != nil {
		return err
	}
	_, err := w.conn.Write(masked)
	return err
}

func (w *wsConn) ReadMessage(timeout time.Duration) ([]byte, error) {
	if timeout > 0 {
		_ = w.conn.SetReadDeadline(time.Now().Add(timeout))
	} else {
		_ = w.conn.SetReadDeadline(time.Time{})
	}

	for {
		b0, err := w.br.ReadByte()
		if err != nil {
			return nil, err
		}
		opcode := b0 & 0x0F

		b1, err := w.br.ReadByte()
		if err != nil {
			return nil, err
		}
		isMasked := (b1 & 0x80) != 0
		lenByte := b1 & 0x7F

		var payloadLen int64
		if lenByte <= 125 {
			payloadLen = int64(lenByte)
		} else if lenByte == 126 {
			var l uint16
			if err := binary.Read(w.br, binary.BigEndian, &l); err != nil {
				return nil, err
			}
			payloadLen = int64(l)
		} else {
			var l uint64
			if err := binary.Read(w.br, binary.BigEndian, &l); err != nil {
				return nil, err
			}
			payloadLen = int64(l)
		}

		var maskKey [4]byte
		if isMasked {
			if _, err := io.ReadFull(w.br, maskKey[:]); err != nil {
				return nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return nil, err
		}

		if isMasked {
			for i := int64(0); i < payloadLen; i++ {
				payload[i] ^= maskKey[i%4]
			}
		}

		if opcode == 0x01 {
			return payload, nil
		}
		if opcode == 0x08 {
			return nil, io.EOF
		}
	}
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}
