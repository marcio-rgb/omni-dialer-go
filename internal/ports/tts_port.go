package ports

import "context"

// TTSPort define o contrato do motor de síntese de fala (Text-to-Speech)
//
// @pattern Adapter (Port Interface)
// @governedBy /docs/rules/ARCHITECT.md
type TTSPort interface {
	// Synthesize sintetiza o texto em um arquivo WAV no caminho de saída especificado
	Synthesize(ctx context.Context, text string, outputPath string) error

	// GetSampleRate retorna a taxa de amostragem padrão do motor (ex: 22050)
	GetSampleRate() int
}
