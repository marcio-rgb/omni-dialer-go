package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"time"
)

const wsMagicGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsConn cliente e servidor WebSocket RFC 6455 nativo em Go puro (Zero Dependências Externas).
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader
}

// UpgradeServerConnection realiza o handshake de entrada RFC 6455 para conexões do Asterisk / vosk-eagi.
func UpgradeServerConnection(conn net.Conn) (*wsConn, error) {
	br := bufio.NewReader(conn)

	reqLine, err := br.ReadString('\n')
	if err != nil {
		return nil, fmt.Errorf("falha ao ler linha de requisicao: %w", err)
	}
	if !strings.HasPrefix(reqLine, "GET ") {
		return nil, fmt.Errorf("metodo invalido, esperado GET: %s", strings.TrimSpace(reqLine))
	}

	secKey := ""
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return nil, fmt.Errorf("falha ao ler headers HTTP: %w", err)
		}
		trimmed := strings.TrimRight(line, "\r\n")
		if trimmed == "" {
			break
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "Sec-WebSocket-Key") {
			secKey = strings.TrimSpace(parts[1])
		}
	}

	if secKey == "" {
		return nil, fmt.Errorf("cabecalho Sec-WebSocket-Key ausente")
	}

	// Calcula hash canônico SHA-1 da chave + Magic GUID
	h := sha1.New()
	h.Write([]byte(secKey + wsMagicGUID))
	acceptKey := base64.StdEncoding.EncodeToString(h.Sum(nil))

	resp := fmt.Sprintf("HTTP/1.1 101 Switching Protocols\r\n"+
		"Upgrade: websocket\r\n"+
		"Connection: Upgrade\r\n"+
		"Sec-WebSocket-Accept: %s\r\n\r\n", acceptKey)

	if _, err := conn.Write([]byte(resp)); err != nil {
		return nil, fmt.Errorf("falha ao enviar resposta 101 Switching Protocols: %w", err)
	}

	return &wsConn{conn: conn, br: br}, nil
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
		_ = conn.Close()
		return nil, err
	}

	br := bufio.NewReader(conn)
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		if line == "\r\n" || line == "\n" {
			break
		}
	}

	_ = conn.SetDeadline(time.Time{})
	return &wsConn{conn: conn, br: br}, nil
}

// WriteServerText envia frame de texto (0x01) sem máscara conforme RFC 6455 para respostas de servidor.
func (w *wsConn) WriteServerText(text string) error {
	payload := []byte(text)
	var header bytes.Buffer
	header.WriteByte(0x80 | 0x01) // FIN + text opcode

	length := len(payload)
	if length <= 125 {
		header.WriteByte(byte(length))
	} else if length <= 65535 {
		header.WriteByte(126)
		_ = binary.Write(&header, binary.BigEndian, uint16(length))
	} else {
		header.WriteByte(127)
		_ = binary.Write(&header, binary.BigEndian, uint64(length))
	}

	if _, err := w.conn.Write(header.Bytes()); err != nil {
		return err
	}
	_, err := w.conn.Write(payload)
	return err
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
	_, payload, err := w.ReadFrame(timeout)
	return payload, err
}

func (w *wsConn) ReadFrame(timeout time.Duration) (byte, []byte, error) {
	if timeout > 0 {
		_ = w.conn.SetReadDeadline(time.Now().Add(timeout))
	} else {
		_ = w.conn.SetReadDeadline(time.Time{})
	}

	for {
		b0, err := w.br.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		opcode := b0 & 0x0F

		b1, err := w.br.ReadByte()
		if err != nil {
			return 0, nil, err
		}
		isMasked := (b1 & 0x80) != 0
		lenByte := b1 & 0x7F

		var payloadLen int64
		if lenByte <= 125 {
			payloadLen = int64(lenByte)
		} else if lenByte == 126 {
			var l uint16
			if err := binary.Read(w.br, binary.BigEndian, &l); err != nil {
				return 0, nil, err
			}
			payloadLen = int64(l)
		} else {
			var l uint64
			if err := binary.Read(w.br, binary.BigEndian, &l); err != nil {
				return 0, nil, err
			}
			payloadLen = int64(l)
		}

		var maskKey [4]byte
		if isMasked {
			if _, err := io.ReadFull(w.br, maskKey[:]); err != nil {
				return 0, nil, err
			}
		}

		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(w.br, payload); err != nil {
			return 0, nil, err
		}

		if isMasked {
			for i := int64(0); i < payloadLen; i++ {
				payload[i] ^= maskKey[i%4]
			}
		}

		// Text frame (0x01) ou Binary frame (0x02)
		if opcode == 0x01 || opcode == 0x02 {
			return opcode, payload, nil
		}
		// Close frame (0x08)
		if opcode == 0x08 {
			return opcode, nil, io.EOF
		}
		// Ping frame (0x09) - responde com Pong (0x0A)
		if opcode == 0x09 {
			_ = w.writeServerPong(payload)
			continue
		}
	}
}

func (w *wsConn) writeServerPong(payload []byte) error {
	var header bytes.Buffer
	header.WriteByte(0x80 | 0x0A) // FIN + pong opcode (0x0A)
	header.WriteByte(byte(len(payload) & 0x7F))
	if _, err := w.conn.Write(header.Bytes()); err != nil {
		return err
	}
	if len(payload) > 0 {
		_, err := w.conn.Write(payload)
		return err
	}
	return nil
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}
