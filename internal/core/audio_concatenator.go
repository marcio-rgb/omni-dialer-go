package core

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
)

// WAVHeader representa o cabeçalho canônico RIFF de 44 bytes para áudio PCM
type WAVHeader struct {
	ChunkID       [4]byte // "RIFF"
	ChunkSize     uint32  // 36 + SubChunk2Size
	Format        [4]byte // "WAVE"
	Subchunk1ID   [4]byte // "fmt "
	Subchunk1Size uint32  // 16
	AudioFormat   uint16  // 1 (PCM)
	NumChannels   uint16  // 1 (Mono) ou 2 (Stereo)
	SampleRate    uint32  // Ex: 22050 ou 8000
	ByteRate      uint32  // SampleRate * NumChannels * BitsPerSample/8
	BlockAlign    uint16  // NumChannels * BitsPerSample/8
	BitsPerSample uint16  // 16
	Subchunk2ID   [4]byte // "data"
	Subchunk2Size uint32  // Tamanho do áudio PCM bruto em bytes
}

// AudioConcatenator gerencia a junção binária de múltiplos arquivos WAV com pausas de silêncio
//
// @pattern Service
// @governedBy /docs/rules/ARCHITECT.md
type AudioConcatenator struct{}

// NewAudioConcatenator instancia o concatenador de áudios WAV
//
// @pattern Service (Constructor)
// @governedBy /docs/rules/ARCHITECT.md
func NewAudioConcatenator() *AudioConcatenator {
	return &AudioConcatenator{}
}

// ParsedWAV encapsula os dados PCM extraídos e os metadados do arquivo WAV
type ParsedWAV struct {
	SampleRate    uint32
	NumChannels   uint16
	BitsPerSample uint16
	PCMData       []byte
}

// ParseWAVFile analisa e extrai o fluxo PCM e valida o formato
func (ac *AudioConcatenator) ParseWAVFile(filePath string) (*ParsedWAV, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("falha ao ler arquivo %s: %w", filePath, err)
	}
	return ac.ParseWAVBytes(data)
}

// ParseWAVBytes lê um buffer de bytes WAV e valida sua assinatura RIFF/WAVE
func (ac *AudioConcatenator) ParseWAVBytes(data []byte) (*ParsedWAV, error) {
	if len(data) < 44 {
		return nil, errors.New("arquivo WAV invalido: tamanho menor que o cabecalho de 44 bytes")
	}

	reader := bytes.NewReader(data)
	var h WAVHeader
	if err := binary.Read(reader, binary.LittleEndian, &h); err != nil {
		return nil, fmt.Errorf("falha ao decodificar cabecalho WAV: %w", err)
	}

	if string(h.ChunkID[:]) != "RIFF" || string(h.Format[:]) != "WAVE" {
		return nil, errors.New("formato invalido: nao e um arquivo RIFF/WAVE")
	}
	if string(h.Subchunk1ID[:]) != "fmt " {
		return nil, errors.New("subchunk fmt ausente ou corrompido")
	}
	if h.AudioFormat != 1 {
		return nil, fmt.Errorf("formato de compressao nao suportado: %d (somente PCM 1 suportado)", h.AudioFormat)
	}

	// Se o header tiver o subchunk "data" na posição padrão (offset 44)
	if string(h.Subchunk2ID[:]) == "data" {
		pcmLen := int(h.Subchunk2Size)
		if 44+pcmLen <= len(data) {
			return &ParsedWAV{
				SampleRate:    h.SampleRate,
				NumChannels:   h.NumChannels,
				BitsPerSample: h.BitsPerSample,
				PCMData:       data[44 : 44+pcmLen],
			}, nil
		}
		// Fallback para todo o restante dos dados
		return &ParsedWAV{
			SampleRate:    h.SampleRate,
			NumChannels:   h.NumChannels,
			BitsPerSample: h.BitsPerSample,
			PCMData:       data[44:],
		}, nil
	}

	// Busca dinâmica por subchunk "data" para arquivos com metadados adicionais
	offset := 12
	for offset+8 <= len(data) {
		chunkTag := string(data[offset : offset+4])
		chunkSize := binary.LittleEndian.Uint32(data[offset+4 : offset+8])
		offset += 8
		if chunkTag == "data" {
			end := offset + int(chunkSize)
			if end > len(data) {
				end = len(data)
			}
			return &ParsedWAV{
				SampleRate:    h.SampleRate,
				NumChannels:   h.NumChannels,
				BitsPerSample: h.BitsPerSample,
				PCMData:       data[offset:end],
			}, nil
		}
		offset += int(chunkSize)
	}

	return nil, errors.New("subchunk data nao encontrado no arquivo WAV")
}

// GenerateSilencePCM produz um buffer de silêncio (zeros) com a duração especificada em milissegundos
func (ac *AudioConcatenator) GenerateSilencePCM(durationMs int, sampleRate uint32, numChannels uint16, bitsPerSample uint16) []byte {
	if durationMs <= 0 {
		return nil
	}
	bytesPerSec := int(sampleRate) * int(numChannels) * (int(bitsPerSample) / 8)
	totalBytes := (bytesPerSec * durationMs) / 1000
	// Garante alinhamento de bloco
	blockAlign := int(numChannels) * (int(bitsPerSample) / 8)
	if totalBytes%blockAlign != 0 {
		totalBytes += blockAlign - (totalBytes % blockAlign)
	}
	return make([]byte, totalBytes)
}

// ConcatenateWAVFiles une uma lista ordenada de caminhos de arquivos WAV intercalados por pausa em ms
//
// @pattern Service (Method)
// @governedBy /docs/rules/ARCHITECT.md
//
// @preExecution
// - Validar que pelo menos um arquivo foi fornecido
// - Validar coerência de sample_rate, canais e profundidade de bits entre todos os arquivos
//
// TrimSilencePCM remove o silêncio residual morto nas pontas do áudio PCM de 16-bit
func (ac *AudioConcatenator) TrimSilencePCM(pcm []byte, threshold int16, sampleRate uint32) []byte {
	if len(pcm) < 2 {
		return pcm
	}

	numSamples := len(pcm) / 2
	samples := make([]int16, numSamples)
	reader := bytes.NewReader(pcm)
	if err := binary.Read(reader, binary.LittleEndian, &samples); err != nil {
		return pcm
	}

	// Margem de segurança de 5ms para evitar cliques e cortes bruscos
	paddingSamples := int((sampleRate * 5) / 1000)

	start := 0
	for i, s := range samples {
		if s > threshold || s < -threshold {
			start = i - paddingSamples
			if start < 0 {
				start = 0
			}
			break
		}
	}

	end := numSamples
	for i := numSamples - 1; i >= 0; i-- {
		if samples[i] > threshold || samples[i] < -threshold {
			end = i + paddingSamples
			if end > numSamples {
				end = numSamples
			}
			break
		}
	}

	if start >= end {
		return pcm
	}

	trimmed := samples[start:end]
	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, trimmed)
	return buf.Bytes()
}

// ConcatenateWAVFiles concatena arquivos WAV aplicando uma pausa uniforme em milissegundos
func (ac *AudioConcatenator) ConcatenateWAVFiles(filePaths []string, pauseBetweenMs int) ([]byte, error) {
	return ac.ConcatenateWAVFilesWithPauses(filePaths, []int{pauseBetweenMs})
}

// ConcatenateWAVFilesWithPauses concatena múltiplos arquivos com pausas customizadas para cada transição e trim de silêncio
//
// @pattern Service (Method)
// @governedBy /docs/rules/ARCHITECT.md
func (ac *AudioConcatenator) ConcatenateWAVFilesWithPauses(filePaths []string, pausesBetweenMs []int) ([]byte, error) {
	if len(filePaths) == 0 {
		return nil, errors.New("nenhum arquivo informado para concatenacao")
	}

	parsedList := make([]*ParsedWAV, 0, len(filePaths))
	var targetSampleRate uint32
	var targetChannels uint16
	var targetBits uint16

	for idx, fp := range filePaths {
		pw, err := ac.ParseWAVFile(fp)
		if err != nil {
			return nil, fmt.Errorf("arquivo #%d (%s) invalido: %w", idx+1, fp, err)
		}
		if idx == 0 {
			targetSampleRate = pw.SampleRate
			targetChannels = pw.NumChannels
			targetBits = pw.BitsPerSample
		} else {
			if pw.SampleRate != targetSampleRate || pw.NumChannels != targetChannels || pw.BitsPerSample != targetBits {
				return nil, fmt.Errorf("incompatibilidade no arquivo #%d (%s): esperado %dHz/%dch/%dbits, recebido %dHz/%dch/%dbits",
					idx+1, fp, targetSampleRate, targetChannels, targetBits, pw.SampleRate, pw.NumChannels, pw.BitsPerSample)
			}
		}
		parsedList = append(parsedList, pw)
	}

	var combinedPCM bytes.Buffer
	for i, pw := range parsedList {
		// Aplica trim do silêncio residual das pontas (threshold 400)
		trimmedPCM := ac.TrimSilencePCM(pw.PCMData, 400, targetSampleRate)
		combinedPCM.Write(trimmedPCM)

		// Adiciona pausa customizada entre arquivos (exceto após o último)
		if i < len(parsedList)-1 {
			currentPauseMs := 0
			if len(pausesBetweenMs) > 0 {
				if i < len(pausesBetweenMs) {
					currentPauseMs = pausesBetweenMs[i]
				} else {
					// Reaproveita a última pausa definida caso faltem itens
					currentPauseMs = pausesBetweenMs[len(pausesBetweenMs)-1]
				}
			}

			if currentPauseMs > 0 {
				silence := ac.GenerateSilencePCM(currentPauseMs, targetSampleRate, targetChannels, targetBits)
				if len(silence) > 0 {
					combinedPCM.Write(silence)
				}
			}
		}
	}

	return ac.BuildWAV(combinedPCM.Bytes(), targetSampleRate, targetChannels, targetBits)
}

// BuildWAV monta o cabeçalho WAV canônico com os dados PCM
func (ac *AudioConcatenator) BuildWAV(pcmData []byte, sampleRate uint32, numChannels uint16, bitsPerSample uint16) ([]byte, error) {
	subchunk2Size := uint32(len(pcmData))
	chunkSize := 36 + subchunk2Size
	bytesPerSample := uint32(bitsPerSample / 8)
	byteRate := sampleRate * uint32(numChannels) * bytesPerSample
	blockAlign := numChannels * uint16(bytesPerSample)

	var h WAVHeader
	copy(h.ChunkID[:], "RIFF")
	h.ChunkSize = chunkSize
	copy(h.Format[:], "WAVE")
	copy(h.Subchunk1ID[:], "fmt ")
	h.Subchunk1Size = 16
	h.AudioFormat = 1
	h.NumChannels = numChannels
	h.SampleRate = sampleRate
	h.ByteRate = byteRate
	h.BlockAlign = blockAlign
	h.BitsPerSample = bitsPerSample
	copy(h.Subchunk2ID[:], "data")
	h.Subchunk2Size = subchunk2Size

	var out bytes.Buffer
	if err := binary.Write(&out, binary.LittleEndian, &h); err != nil {
		return nil, fmt.Errorf("falha ao serializar cabecalho WAV: %w", err)
	}
	if _, err := io.Copy(&out, bytes.NewReader(pcmData)); err != nil {
		return nil, fmt.Errorf("falha ao concatenar PCM: %w", err)
	}

	return out.Bytes(), nil
}
