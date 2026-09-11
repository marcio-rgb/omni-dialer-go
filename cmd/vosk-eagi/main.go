package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Frases inequívocas de caixas postais, correio de voz e operadoras
var voicemailPhrases = []string{
	"caixa postal",
	"deixe seu recado",
	"deixe sua mensagem",
	"apos o sinal",
	"apos o bip",
	"nao pode atender",
	"nao pode receber chamadas",
	"impossibilitado de atender",
	"esta impossibilitado",
	"nao esta disponivel",
	"chamada encaminhada",
	"chamada esta sendo",
	"secretaria eletronica",
	"mensagem gravada",
	"horario de atendimento",
	"vivo informa",
	"claro informa",
	"tim informa",
	"numero chamado",
	"o numero para o qual",
	"este numero de telefone",
	"o telefone que voce",
	"desligue a chamada",
	"desligue o telefone",
	"nao esta recebendo chamadas",
	"sua chamada foi completada",
	"programado para nao receber",
	"caixa de mensagens",
	"sua mensagem",
}

// Saudações humanas habituais em telefonia brasileira
var humanGreetings = []string{
	"alo",
	"ola",
	"oi",
	"sim",
	"pronto",
	"pois nao",
	"quem fala",
	"quem e",
	"quem ta falando",
	"fala",
	"opa",
	"bom dia",
	"boa tarde",
	"boa noite",
	"pode falar",
	"com quem",
	"aqui e",
}

type VoskMessage struct {
	Text    string `json:"text"`
	Partial string `json:"partial"`
}

func main() {
	readAGIEnvironment()

	audioFile := os.NewFile(3, "eagi_audio")
	if audioFile == nil {
		agiSetVar("VOSK_AMD_STATUS", "HUMAN")
		agiSetVar("VOSK_AMD_CAUSE", "NO_EAGI_FD3")
		agiSetVar("VOSK_AMD_TEXT", "")
		agiVerbose("VOSK-EAGI: FD 3 indisponivel. Fallback seguro para HUMAN.", 1)
		return
	}
	defer audioFile.Close()

	wsURL := os.Getenv("VOSK_SERVER_URL")
	if wsURL == "" {
		wsURL = "ws://127.0.0.1:2700"
	}

	maxDuration := 2.8
	if envDur := os.Getenv("VOSK_MAX_DURATION_SEC"); envDur != "" {
		if d, err := strconv.ParseFloat(envDur, 64); err == nil && d > 0 {
			maxDuration = d
		}
	}

	wsClient, err := connectWebSocket(wsURL, 1500*time.Millisecond)
	if err != nil {
		agiVerbose(fmt.Sprintf("VOSK-EAGI: Conexao com Vosk falhou: %v. Fallback seguro para HUMAN.", err), 1)
		agiSetVar("VOSK_AMD_STATUS", "HUMAN")
		agiSetVar("VOSK_AMD_CAUSE", "VOSK_CONN_FALLBACK")
		agiSetVar("VOSK_AMD_TEXT", "")
		return
	}
	defer wsClient.Close()

	_ = wsClient.WriteText(`{"config" : { "sample_rate" : 8000 }}`)

	status := "HUMAN"
	cause := "DEFAULT_SAFE_HUMAN"
	fullText := ""
	startTime := time.Now()

	chunkBuf := make([]byte, 1600) // 100ms de áudio PCM 16-bit 8kHz

	for time.Since(startTime) < time.Duration(maxDuration*float64(time.Second)) {
		n, readErr := audioFile.Read(chunkBuf)
		if n > 0 {
			if writeErr := wsClient.WriteBinary(chunkBuf[:n]); writeErr != nil {
				break
			}
		}
		if readErr != nil {
			break
		}

		// Tenta ler mensagem do Vosk (timeout curto para não bloquear o streaming de áudio)
		rawMsg, err := wsClient.ReadMessage(50 * time.Millisecond)
		if err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				txt := normalizeText(vm.Text)
				part := normalizeText(vm.Partial)

				current := txt
				if current == "" {
					current = part
				}

				if txt != "" && !strings.Contains(fullText, txt) {
					fullText = strings.TrimSpace(fullText + " " + txt)
				}

				// 1. Checagem de Caixa Postal
				for _, phrase := range voicemailPhrases {
					if strings.Contains(current, phrase) || strings.Contains(fullText, phrase) {
						status = "MACHINE"
						cause = "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_"))
						break
					}
				}

				if status == "MACHINE" {
					break
				}

				// 2. Checagem de Saudação Humana Imediata ("Alô", "Oi", etc.)
				for _, greeting := range humanGreetings {
					if strings.Contains(current, greeting) || strings.Contains(fullText, greeting) {
						status = "HUMAN"
						cause = "HUMAN_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_"))
						break
					}
				}

				if status == "HUMAN" && strings.HasPrefix(cause, "HUMAN_") {
					// Saudação detectada! Encerra imediatamente para conectar o cliente sem latência
					break
				}
			}
		}
	}

	// Se não for máquina, finaliza para capturar última palavra
	if status != "MACHINE" {
		_ = wsClient.WriteText(`{"eof" : 1}`)
		if rawMsg, err := wsClient.ReadMessage(300 * time.Millisecond); err == nil && len(rawMsg) > 0 {
			var vm VoskMessage
			if jsonErr := json.Unmarshal(rawMsg, &vm); jsonErr == nil {
				txt := normalizeText(vm.Text)
				if txt != "" && !strings.Contains(fullText, txt) {
					fullText = strings.TrimSpace(fullText + " " + txt)
				}
			}
		}

		normFull := normalizeText(fullText)
		for _, phrase := range voicemailPhrases {
			if strings.Contains(normFull, phrase) {
				status = "MACHINE"
				cause = "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_"))
				break
			}
		}

		if status != "MACHINE" {
			for _, greeting := range humanGreetings {
				if strings.Contains(normFull, greeting) {
					status = "HUMAN"
					cause = "HUMAN_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_"))
					break
				}
			}
		}

		if status != "MACHINE" && normFull != "" {
			status = "HUMAN"
			cause = "HUMAN_NATURAL_SPEECH"
		}
	}

	agiVerbose(fmt.Sprintf("VOSK-EAGI Concluido: STATUS=%s CAUSA=%s TEXTO='%s'", status, cause, fullText), 1)
	agiSetVar("VOSK_AMD_STATUS", status)
	agiSetVar("VOSK_AMD_CAUSE", cause)
	agiSetVar("VOSK_AMD_TEXT", fullText)
}

func readAGIEnvironment() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
	}
}

func agiSend(cmd string) {
	fmt.Fprintf(os.Stdout, "%s\n", cmd)
}

func agiSetVar(name, value string) {
	agiSend(fmt.Sprintf("SET VARIABLE %s %q", name, value))
}

func agiVerbose(msg string, level int) {
	agiSend(fmt.Sprintf("VERBOSE %q %d", msg, level))
}

func normalizeText(s string) string {
	if s == "" {
		return ""
	}
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, err := transform.String(t, s)
	if err != nil {
		res = s
	}
	return strings.ToLower(strings.TrimSpace(res))
}

// --- Cliente WebSocket Nativo RFC 6455 Ultrarrápido (Zero Dependências Externas) ---

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

		// Text frame (0x01)
		if opcode == 0x01 {
			return payload, nil
		}
		// Close frame (0x08)
		if opcode == 0x08 {
			return nil, io.EOF
		}
	}
}

func (w *wsConn) Close() error {
	return w.conn.Close()
}
