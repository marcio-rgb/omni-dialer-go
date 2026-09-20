package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

/**
 * Utilitário para leitura e manipulação de arquivos WAV de telefonia (8000 Hz / 16-bit).
 *
 * @pattern Utility / Adapter
 * @governedBy .agents/ARCHITECT.md
 */

type WavAudio struct {
	SampleRate    uint32
	BitsPerSample uint16
	NumChannels   uint16
	Data          []byte // Dados PCM brutos intercalados
}

func ReadWav(path string) (*WavAudio, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) < 44 {
		return nil, fmt.Errorf("arquivo WAV truncado: %d bytes", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("formato WAV inválido")
	}

	reader := bytes.NewReader(data[12:])
	var sampleRate uint32 = 8000
	var bitsPerSample uint16 = 16
	var numChannels uint16 = 1
	var audioData []byte

	for reader.Len() >= 8 {
		var chunkID [4]byte
		var chunkSize uint32
		if err := binary.Read(reader, binary.BigEndian, &chunkID); err != nil {
			break
		}
		if err := binary.Read(reader, binary.LittleEndian, &chunkSize); err != nil {
			break
		}

		chunkName := string(chunkID[:])
		if chunkName == "fmt " {
			var audioFormat uint16
			var byteRate uint32
			var blockAlign uint16
			binary.Read(reader, binary.LittleEndian, &audioFormat)
			binary.Read(reader, binary.LittleEndian, &numChannels)
			binary.Read(reader, binary.LittleEndian, &sampleRate)
			binary.Read(reader, binary.LittleEndian, &byteRate)
			binary.Read(reader, binary.LittleEndian, &blockAlign)
			binary.Read(reader, binary.LittleEndian, &bitsPerSample)

			if chunkSize > 16 {
				reader.Seek(int64(chunkSize-16), io.SeekCurrent)
			}
		} else if chunkName == "data" {
			audioData = make([]byte, chunkSize)
			n, _ := reader.Read(audioData)
			audioData = audioData[:n]
			break
		} else {
			reader.Seek(int64(chunkSize), io.SeekCurrent)
		}
	}

	return &WavAudio{
		SampleRate:    sampleRate,
		BitsPerSample: bitsPerSample,
		NumChannels:   numChannels,
		Data:          audioData,
	}, nil
}

// SplitStereoToMono separa Canal 1 (Left / TX Atendente) e Canal 2 (Right / RX Cliente)
func SplitStereoToMono(w *WavAudio) (leftMono, rightMono []byte, err error) {
	if w.NumChannels != 2 {
		return nil, nil, fmt.Errorf("áudio não é estéreo (canais: %d)", w.NumChannels)
	}

	sampleSize := 2 // 16-bit
	frameSize := sampleSize * 2
	numFrames := len(w.Data) / frameSize

	left := make([]byte, numFrames*sampleSize)
	right := make([]byte, numFrames*sampleSize)

	for i := 0; i < numFrames; i++ {
		offset := i * frameSize
		copy(left[i*sampleSize:(i+1)*sampleSize], w.Data[offset:offset+sampleSize])
		copy(right[i*sampleSize:(i+1)*sampleSize], w.Data[offset+sampleSize:offset+frameSize])
	}

	return left, right, nil
}

// WriteMonoWav grava áudio mono 16-bit PCM em arquivo WAV
func WriteMonoWav(path string, sampleRate uint32, pcmData []byte) error {
	totalDataSize := uint32(len(pcmData))
	totalFileSize := 36 + totalDataSize

	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, totalFileSize)
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16)) // PCM chunk size
	binary.Write(buf, binary.LittleEndian, uint16(1))  // AudioFormat = PCM
	binary.Write(buf, binary.LittleEndian, uint16(1))  // NumChannels = 1 (Mono)
	binary.Write(buf, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * 1 * 2
	binary.Write(buf, binary.LittleEndian, byteRate)
	binary.Write(buf, binary.LittleEndian, uint16(2))  // BlockAlign = 2
	binary.Write(buf, binary.LittleEndian, uint16(16)) // BitsPerSample = 16

	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, totalDataSize)
	buf.Write(pcmData)

	return os.WriteFile(path, buf.Bytes(), 0644)
}

// TrimWavAudio descarta os primeiros startOffsetMs de áudio para remover prompts institucionais (TX).
func TrimWavAudio(w *WavAudio, startOffsetMs int) (*WavAudio, error) {
	if w == nil {
		return nil, fmt.Errorf("audio nulo")
	}
	if startOffsetMs <= 0 {
		return w, nil
	}

	bytesPerMs := (int(w.SampleRate) * int(w.NumChannels) * int(w.BitsPerSample/8)) / 1000
	offsetBytes := startOffsetMs * bytesPerMs

	// Se o áudio for menor ou igual ao offset, não há sinal de cliente pós-prompt
	if len(w.Data) <= offsetBytes {
		return nil, fmt.Errorf("audio curto demais para corte (%d bytes <= offset %d bytes)", len(w.Data), offsetBytes)
	}

	trimmedData := make([]byte, len(w.Data)-offsetBytes)
	copy(trimmedData, w.Data[offsetBytes:])

	return &WavAudio{
		SampleRate:    w.SampleRate,
		BitsPerSample: w.BitsPerSample,
		NumChannels:   w.NumChannels,
		Data:          trimmedData,
	}, nil
}

