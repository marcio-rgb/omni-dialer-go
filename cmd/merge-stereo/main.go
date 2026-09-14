package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

/**
 * Utilitário autônomo para fusão de áudios mono (TX e RX) em áudio estéreo dual-channel.
 * Canal 1 (Left / Esquerdo): TX - Voz do Atendente / Usuário / Sistema
 * Canal 2 (Right / Direito): RX - Voz do Cliente / Lead
 *
 * @pattern Utility / Adapter
 * @governedBy .agents/skills/telephony-livekit-dialer/SKILL.md#gravacao-estereo-dual-channel
 */

type wavInfo struct {
	sampleRate    uint32
	bitsPerSample uint16
	audioData     []byte
}

func parseWav(filePath string) (*wavInfo, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	if len(data) < 44 {
		return nil, fmt.Errorf("arquivo WAV muito pequeno (%d bytes)", len(data))
	}
	if string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, fmt.Errorf("formato WAV/RIFF invalido")
	}

	reader := bytes.NewReader(data[12:])
	var sampleRate uint32 = 8000
	var bitsPerSample uint16 = 16
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
			var audioFormat, numChannels uint16
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

	return &wavInfo{
		sampleRate:    sampleRate,
		bitsPerSample: bitsPerSample,
		audioData:     audioData,
	}, nil
}

func writeStereoWav(outFile string, sampleRate uint32, leftData, rightData []byte) error {
	sampleSize := 2 // 16 bits = 2 bytes
	maxLen := len(leftData)
	if len(rightData) > maxLen {
		maxLen = len(rightData)
	}

	if maxLen%sampleSize != 0 {
		maxLen += sampleSize - (maxLen % sampleSize)
	}

	stereoDataSize := uint32(maxLen * 2)
	totalFileSize := 36 + stereoDataSize

	buf := new(bytes.Buffer)
	buf.WriteString("RIFF")
	binary.Write(buf, binary.LittleEndian, totalFileSize)
	buf.WriteString("WAVE")

	buf.WriteString("fmt ")
	binary.Write(buf, binary.LittleEndian, uint32(16))
	binary.Write(buf, binary.LittleEndian, uint16(1))
	binary.Write(buf, binary.LittleEndian, uint16(2))
	binary.Write(buf, binary.LittleEndian, sampleRate)
	byteRate := sampleRate * 2 * 2
	binary.Write(buf, binary.LittleEndian, byteRate)
	binary.Write(buf, binary.LittleEndian, uint16(4))
	binary.Write(buf, binary.LittleEndian, uint16(16))

	buf.WriteString("data")
	binary.Write(buf, binary.LittleEndian, stereoDataSize)

	for i := 0; i < maxLen; i += sampleSize {
		if i+sampleSize <= len(leftData) {
			buf.Write(leftData[i : i+sampleSize])
		} else {
			buf.Write([]byte{0, 0})
		}

		if i+sampleSize <= len(rightData) {
			buf.Write(rightData[i : i+sampleSize])
		} else {
			buf.Write([]byte{0, 0})
		}
	}

	tmpFile := outFile + ".tmp"
	if err := os.WriteFile(tmpFile, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("erro ao salvar arquivo temporario stereo: %w", err)
	}

	if err := os.Rename(tmpFile, outFile); err != nil {
		return fmt.Errorf("erro ao renomear arquivo final: %w", err)
	}

	return nil
}

func main() {
	if len(os.Args) < 4 {
		fmt.Printf("Uso: %s <tx_mono.wav> <rx_mono.wav> <output_stereo.wav>\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}

	txFile := os.Args[1]
	rxFile := os.Args[2]
	outFile := os.Args[3]

	var txInfo, rxInfo *wavInfo
	var sampleRate uint32 = 8000

	if t, err := parseWav(txFile); err == nil {
		txInfo = t
		if t.sampleRate > 0 {
			sampleRate = t.sampleRate
		}
	} else {
		fmt.Fprintf(os.Stderr, "Aviso: Nao foi possivel ler TX (%s): %v\n", txFile, err)
	}

	if r, err := parseWav(rxFile); err == nil {
		rxInfo = r
		if r.sampleRate > 0 && txInfo == nil {
			sampleRate = r.sampleRate
		}
	} else {
		fmt.Fprintf(os.Stderr, "Aviso: Nao foi possivel ler RX (%s): %v\n", rxFile, err)
	}

	var leftData, rightData []byte
	if txInfo != nil {
		leftData = txInfo.audioData
	}
	if rxInfo != nil {
		rightData = rxInfo.audioData
	}

	if len(leftData) == 0 && len(rightData) == 0 {
		fmt.Fprintf(os.Stderr, "Erro: Ambos arquivos TX e RX estao vazios ou ausentes\n")
		os.Exit(2)
	}

	if err := writeStereoWav(outFile, sampleRate, leftData, rightData); err != nil {
		fmt.Fprintf(os.Stderr, "Falha ao gravar stereo: %v\n", err)
		os.Exit(3)
	}

	if txFile != outFile {
		_ = os.Remove(txFile)
	}
	if rxFile != outFile {
		_ = os.Remove(rxFile)
	}

	fmt.Printf("Sucesso: Audio estereo gerado em %s (Taxa: %d Hz)\n", outFile, sampleRate)
}
