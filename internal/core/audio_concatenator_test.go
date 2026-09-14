package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAudioConcatenator_BuildAndConcatenate(t *testing.T) {
	ac := NewAudioConcatenator()

	// Gera 2 buffers de áudio PCM sintéticos (100ms cada a 22050Hz, 16 bits, mono)
	sampleRate := uint32(22050)
	numChannels := uint16(1)
	bitsPerSample := uint16(16)
	bytesPerSample := uint32(bitsPerSample / 8)

	numSamples100ms := (sampleRate * 100) / 1000
	pcm1 := make([]byte, numSamples100ms*bytesPerSample)
	for i := range pcm1 {
		pcm1[i] = 0x55 // Padrão de teste
	}
	pcm2 := make([]byte, numSamples100ms*bytesPerSample)
	for i := range pcm2 {
		pcm2[i] = 0xAA // Padrão de teste
	}

	wav1Bytes, err := ac.BuildWAV(pcm1, sampleRate, numChannels, bitsPerSample)
	if err != nil {
		t.Fatalf("falha ao criar wav1: %v", err)
	}
	wav2Bytes, err := ac.BuildWAV(pcm2, sampleRate, numChannels, bitsPerSample)
	if err != nil {
		t.Fatalf("falha ao criar wav2: %v", err)
	}

	tmpDir := t.TempDir()
	f1 := filepath.Join(tmpDir, "w1.wav")
	f2 := filepath.Join(tmpDir, "w2.wav")

	if err := os.WriteFile(f1, wav1Bytes, 0644); err != nil {
		t.Fatalf("falha ao gravar f1: %v", err)
	}
	if err := os.WriteFile(f2, wav2Bytes, 0644); err != nil {
		t.Fatalf("falha ao gravar f2: %v", err)
	}

	// Concatena com pausa de 50ms
	pauseMs := 50
	combinedWAV, err := ac.ConcatenateWAVFiles([]string{f1, f2}, pauseMs)
	if err != nil {
		t.Fatalf("falha na concatenacao: %v", err)
	}

	parsed, err := ac.ParseWAVBytes(combinedWAV)
	if err != nil {
		t.Fatalf("falha ao re-analisar WAV combinado: %v", err)
	}

	silenceBytes := len(ac.GenerateSilencePCM(pauseMs, sampleRate, numChannels, bitsPerSample))
	expectedTotalBytes := len(pcm1) + silenceBytes + len(pcm2)

	if len(parsed.PCMData) != expectedTotalBytes {
		t.Errorf("tamanho PCM inesperado: obtido %d, esperado %d", len(parsed.PCMData), expectedTotalBytes)
	}
	if parsed.SampleRate != sampleRate {
		t.Errorf("sample_rate inesperado: obtido %d, esperado %d", parsed.SampleRate, sampleRate)
	}
}
