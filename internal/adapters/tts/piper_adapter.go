package tts

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"dialer-go/internal/ports"
)

// PiperAdapter implementa a porta TTSPort executando o binário Piper local com modelo ONNX
//
// @pattern Adapter
// @governedBy /docs/rules/ARCHITECT.md
type PiperAdapter struct {
	binPath    string
	modelPath  string
	configPath string
	sampleRate int
	mu         sync.Mutex
}

// NewPiperAdapter instancia o adaptador com os caminhos do binário e modelo
//
// @pattern Adapter (Constructor)
// @governedBy /docs/rules/ARCHITECT.md
func NewPiperAdapter(binPath, modelPath, configPath string) (ports.TTSPort, error) {
	if binPath == "" {
		binPath = "./bin/piper/piper"
	}
	if modelPath == "" {
		modelPath = "./models/piper/dii_pt-BR.onnx"
	}
	if configPath == "" {
		configPath = "./models/piper/dii_pt-BR.onnx.json"
	}

	if _, err := os.Stat(binPath); err != nil {
		return nil, fmt.Errorf("binario do piper nao encontrado em %s: %w", binPath, err)
	}
	if _, err := os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("modelo ONNX do piper nao encontrado em %s: %w", modelPath, err)
	}
	if _, err := os.Stat(configPath); err != nil {
		return nil, fmt.Errorf("configuracao do modelo ONNX nao encontrada em %s: %w", configPath, err)
	}

	return &PiperAdapter{
		binPath:    binPath,
		modelPath:  modelPath,
		configPath: configPath,
		sampleRate: 22050,
	}, nil
}

// Synthesize executa a síntese da frase no arquivo de saída
//
// @pattern Adapter (Method)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Validar parâmetros de texto e caminho de saída
// - Garantir que o diretório pai do arquivo de saída existe
//
// @postExecution
// - Grava o arquivo WAV em disco e valida que o arquivo foi gerado com tamanho > 0
func (a *PiperAdapter) Synthesize(ctx context.Context, text string, outputPath string) error {
	if text == "" {
		return fmt.Errorf("texto para sintese nao pode ser vazio")
	}

	// Garante que o diretório pai existe
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("falha ao criar diretorio para %s: %w", outputPath, err)
	}

	// Trava para evitar sobrecarga de concorrência excessiva no processo
	a.mu.Lock()
	defer a.mu.Unlock()

	timeoutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx, a.binPath,
		"-m", a.modelPath,
		"-c", a.configPath,
		"-f", outputPath,
		"-q",
	)

	cmd.Stdin = bytes.NewBufferString(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("piper execution failed: %w, stderr: %s", err, stderr.String())
	}

	info, err := os.Stat(outputPath)
	if err != nil || info.Size() == 0 {
		return fmt.Errorf("arquivo gerado %s esta vazio ou inacessivel", outputPath)
	}

	return nil
}

// GetSampleRate retorna a taxa de amostragem padrão (22050 Hz)
func (a *PiperAdapter) GetSampleRate() int {
	return a.sampleRate
}
