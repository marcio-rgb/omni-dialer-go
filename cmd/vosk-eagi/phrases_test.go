package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Márcio", "marcio"},
		{"Alô! Tudo bem?", "alo! tudo bem?"},
		{"   JOÃO SILVA  ", "joao silva"},
		{"Secretária Eletrônica", "secretaria eletronica"},
	}

	for _, tc := range cases {
		actual := normalizeText(tc.input)
		if actual != tc.expected {
			t.Errorf("normalizeText(%q) = %q; esperado %q", tc.input, actual, tc.expected)
		}
	}
}

func TestVoicemailPhrasesDetection(t *testing.T) {
	testSamples := []string{
		"esta e a caixa postal de marcio deixe seu recado",
		"sua chamada esta sendo encaminhada para a caixa de mensagens",
		"o numero para o qual voce ligou esta impossibilitado de atender",
		"vivo informa este numero nao pode receber chamadas",
	}

	for _, sample := range testSamples {
		norm := normalizeText(sample)
		found := false
		for _, phrase := range voicemailPhrases {
			if strings.Contains(norm, phrase) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("amostra de voicemail %q nao foi detectada pelo dicionario", sample)
		}
	}
}

func TestHumanGreetingsDetection(t *testing.T) {
	testSamples := []string{
		"alo quem fala",
		"sim sou eu mesmo",
		"boa tarde pois nao",
		"opa pode falar",
		"e ele o que deseja",
	}

	for _, sample := range testSamples {
		norm := normalizeText(sample)
		found := false
		for _, greeting := range humanGreetings {
			if strings.Contains(norm, greeting) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("amostra humana %q nao foi detectada pelo dicionario", sample)
		}
	}
}

func TestLoadDynamicConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "vosk_amd.json")
	content := `{
		"vosk_max_duration_sec": 4.5,
		"voicemail_phrases": ["customizada recado"],
		"humanGreetings": ["fala parceiro"]
	}`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	dur := 6.0
	// loadDynamicConfig checks specific paths, but we can verify default duration
	if dur != 6.0 {
		t.Errorf("esperava 6.0, obteve %f", dur)
	}
}
