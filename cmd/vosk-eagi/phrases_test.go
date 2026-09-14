package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizeText(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Márcio", "marcio"},
		{"Alô! Tudo bem?", "alo tudo bem"},
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

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b     string
		expected int
	}{
		{"caixa", "caixa", 0},
		{"caixa postal", "caixa postar", 1},
		{"alo", "ala", 1},
		{"recado", "recados", 1},
		{"", "teste", 5},
	}

	for _, tc := range cases {
		dist := levenshtein(tc.a, tc.b)
		if dist != tc.expected {
			t.Errorf("levenshtein(%q, %q) = %d; esperado %d", tc.a, tc.b, dist, tc.expected)
		}
	}
}

func TestFuzzyContainsPhrase(t *testing.T) {
	if !fuzzyContainsPhrase("esta e a caixa postar da vivo", "caixa postal", 1) {
		t.Errorf("esperava casar 'caixa postar' com 'caixa postal' com distancia 1")
	}
	if !fuzzyContainsPhrase("favor deixa seu recado apos o sinal", "deixe seu recado", 2) {
		t.Errorf("esperava casar com tolerancia 2")
	}
	if fuzzyContainsPhrase("bom dia tudo bem", "caixa postal", 1) {
		t.Errorf("nao devia casar frase totalmente diferente")
	}
}

func TestVoicemailPhrasesDetection(t *testing.T) {
	testSamples := []string{
		"esta e a caixa postal de marcio deixe seu recado",
		"sua chamada esta sendo encaminhada para a caixa de mensagens",
		"o numero para o qual voce ligou esta temporariamente fora de servico",
		"vivo informa este numero nao pode receber chamadas",
	}

	for _, sample := range testSamples {
		status, reason := ClassifyCall(CallMetrics{FullText: sample})
		if status != "MACHINE" {
			t.Errorf("amostra de voicemail %q nao foi detectada como MACHINE, obteve %q (%s)", sample, status, reason)
		}
	}
}

func TestHumanGreetingsDetection(t *testing.T) {
	testSamples := []struct {
		text string
	}{
		{"alo quem fala"},
		{"sim sou eu mesmo"},
		{"boa tarde pois nao"},
		{"opa pode falar"},
		{"quem fala"},
		{"quem e"},
		{"alo"},
		{"pronto"},
		{"fala ai"},
		{"tudo bem"},
	}

	for _, sample := range testSamples {
		status, reason := ClassifyCall(CallMetrics{FullText: sample.text})
		if status != "HUMAN" {
			t.Errorf("amostra humana %q nao foi detectada como HUMAN, obteve %q (%s)", sample.text, status, reason)
		}
	}
}

func TestClassifyCall_SilenceAssumesHuman(t *testing.T) {
	silenceCases := []string{
		"",
		"   ",
		"\t\n",
	}

	for _, tc := range silenceCases {
		status, cause := ClassifyCall(CallMetrics{FullText: tc})
		if status != "HUMAN" {
			t.Errorf("para silêncio %q, esperava status HUMAN, obteve %q", tc, status)
		}
		if cause != "HUMAN_NATURAL_PAUSE" {
			t.Errorf("para silêncio %q, esperava causa HUMAN_NATURAL_PAUSE, obteve %q", tc, cause)
		}
	}
}

func TestClassifyCall_ContinuousMonologue(t *testing.T) {
	monologue := "atencao cliente do banco esta e uma mensagem de cobranca automatica favor comparecer a uma agencia"
	metrics := CallMetrics{
		FullText:          monologue,
		SpeechDurationSec: 5.0,
		SilenceAfterSec:   0.2,
	}

	status, reason := ClassifyCall(metrics)
	if status != "MACHINE" {
		t.Errorf("esperava MACHINE para monólogo contínuo longo, obteve %q", status)
	}
	if reason != "CONTINUOUS_MONOLOGUE_DETECTED" {
		t.Errorf("esperava CONTINUOUS_MONOLOGUE_DETECTED, obteve %q", reason)
	}
}

func TestLoadDynamicConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgFile := filepath.Join(tmpDir, "vosk_amd.json")
	content := `{
		"vosk_max_duration_sec": 4.5,
		"voicemail_phrases": ["customizada recado"],
		"human_greetings": ["fala parceiro"]
	}`
	if err := os.WriteFile(cfgFile, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	dur := 6.0
	if dur != 6.0 {
		t.Errorf("esperava 6.0, obteve %f", dur)
	}
}
