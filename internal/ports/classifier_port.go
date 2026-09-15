package ports

import (
	"context"
	"io"

	"dialer-go/internal/domain"
)

// SpeechRecognizerPort define o contrato agnóstico para motores de reconhecimento acústico (STT).
// Qualquer tecnologia (Vosk, Kaldi, Whisper, Silero, Riva) deve implementar esta interface.
//
// @pattern Port (Hexagonal Architecture)
// @governedBy .agents/ARCHITECT.md
type SpeechRecognizerPort interface {
	// AcceptAudio alimenta o motor com dados PCM lineares brutos.
	AcceptAudio(frame domain.AudioFrame) error

	// GetPartial retorna a hipótese textual corrente em tempo real.
	GetPartial() string

	// GetFinal retorna o texto consolidado final quando um silêncio ou pontuação é detectado.
	GetFinal() string

	// Reset limpa buffers internos preparando para uma nova chamada.
	Reset()

	// Close libera recursos alocados pelo motor acústico.
	Close() error
}

// SemanticClassifierPort define o contrato de análise semântica e decisão telefônica.
//
// @pattern Strategy Pattern
// @governedBy .agents/ARCHITECT.md
type SemanticClassifierPort interface {
	// Evaluate avalia incrementalmente o texto recebido e durações de áudio.
	// Retorna o veredito e true se a classificação atingiu certeza (Fast-Exit), ou false se inconclusiva.
	Evaluate(partialText, finalText string, speechSec, silenceSec float64) (domain.ClassificationVerdict, bool)

	// Finalize força a emissão do veredito final ao término do tempo de áudio ou stream.
	Finalize(finalText string, speechSec, silenceSec float64) domain.ClassificationVerdict
}

// EventSink define o contrato de emissão de eventos assíncronos em tempo real da sessão.
//
// @pattern Observer / Consumer
// @governedBy .agents/ARCHITECT.md
type EventSink interface {
	// EmitEvent envia um evento estruturado para o cliente (Asterisk/ThinClient).
	EmitEvent(msg domain.WireEventMessage) error
}

// ClassificationEnginePort orquestra a sessão completa de uma chamada telefônica.
//
// @pattern Facade / Pipeline
// @governedBy .agents/ARCHITECT.md
type ClassificationEnginePort interface {
	// ProcessSession processa o fluxo contínuo de áudio da chamada até a emissão do veredito.
	ProcessSession(ctx context.Context, audioReader io.Reader, sink EventSink) (domain.ClassificationVerdict, error)
}
