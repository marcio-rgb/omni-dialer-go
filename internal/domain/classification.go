package domain

import "time"

// ClassificationStatus define o veredito canônico da chamada.
//
// @pattern Value Object / Domain Enum
// @governedBy .agents/ARCHITECT.md
type ClassificationStatus string

const (
	StatusHuman   ClassificationStatus = "HUMAN"
	StatusMachine ClassificationStatus = "MACHINE"
	StatusUnknown ClassificationStatus = "UNKNOWN"
)

// ClassificationVerdict representa o resultado final estruturado e imutável de uma classificação telefônica.
//
// @pattern Value Object (Imutável)
// @governedBy .agents/ARCHITECT.md
type ClassificationVerdict struct {
	Status        ClassificationStatus `json:"status"`         // HUMAN ou MACHINE
	Cause         string               `json:"cause"`          // HUMAN_GREETING_ALO, VOICEMAIL_MATCH_..., etc.
	Transcription string               `json:"transcription"`  // Texto normalizado reconhecido
	Confidence    float64              `json:"confidence"`     // Grau de confiança (0.0 a 1.0)
	LatencyMs     int64                `json:"latency_ms"`     // Tempo decorrido até a classificação em milissegundos
	SpeechSec     float64              `json:"speech_sec"`     // Duração do sinal de voz detectado em segundos
	SilenceSec    float64              `json:"silence_sec"`    // Silêncio após a fala em segundos
	CreatedAt     time.Time            `json:"created_at"`     // Timestamp de emissão do veredito
}

// AudioFrame encapsula um pacote linear de áudio bruto recebido da telefonia.
//
// @pattern Value Object
// @governedBy .agents/ARCHITECT.md
type AudioFrame struct {
	SampleRate int    `json:"sample_rate"` // Padrão: 8000 Hz nativo telefônico
	Channels   int    `json:"channels"`    // Padrão: 1 (mono)
	PCMData    []byte `json:"-"`           // Buffer PCM linear 16-bit
}

// WireActionType define as mensagens trafegadas no wire protocol WebSocket.
type WireActionType string

const (
	WireActionStart             WireActionType = "start"
	WireEventTranscriptionPart  WireActionType = "transcription_partial"
	WireEventTranscriptionFinal WireActionType = "transcription_final"
	WireEventVerdict            WireActionType = "classification_verdict"
	WireEventHeartbeat          WireActionType = "ping"
)

// WireHandshakeMessage define a carga inicial enviada pelo cliente telefônico ao abrir a sessão.
type WireHandshakeMessage struct {
	Action         WireActionType `json:"action"`
	SampleRate     int            `json:"sample_rate"`
	MaxDurationSec float64        `json:"max_duration_sec"`
	ChannelID      string         `json:"channel_id,omitempty"`
}

// WireEventMessage define a estrutura de evento enviada em tempo real pelo Classificator.
type WireEventMessage struct {
	Type          WireActionType         `json:"type"`
	Text          string                 `json:"text,omitempty"`
	Status        ClassificationStatus   `json:"status,omitempty"`
	Cause         string                 `json:"cause,omitempty"`
	LatencyMs     int64                  `json:"latency_ms,omitempty"`
	Confidence    float64                `json:"confidence,omitempty"`
	VerdictDetail *ClassificationVerdict `json:"verdict_detail,omitempty"`
}
