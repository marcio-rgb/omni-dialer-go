package main

import (
	"encoding/json"
	"os"
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// Frases inequívocas de caixas postais, correio de voz e operadoras
var voicemailPhrases = []string{
	"caixa postal",
	"deixe recado",
	"deixe seu recado",
	"recado",
	"assim que possivel",
	"deixe sua mensagem",
	"apos o sinal",
	"apos o bip",
	"nao pode atender",
	"nao pode receber chamadas",
	"impossibilitado de atender",
	"esta impossibilitado",
	"nao esta disponivel",
	"chamada encaminhada",
	"chamada esta sendo",
	"secretaria eletronica",
	"mensagem gravada",
	"horario de atendimento",
	"vivo informa",
	"claro informa",
	"tim informa",
	"numero chamado",
	"o numero para o qual",
	"este numero de telefone",
	"o telefone que voce",
	"desligue a chamada",
	"desligue o telefone",
	"nao esta recebendo chamadas",
	"sua chamada foi completada",
	"programado para nao receber",
	"caixa de mensagens",
	"sua mensagem",
}

// Saudações e confirmações humanas habituais em telefonia brasileira (Avaliação Positiva de Atendimento)
var humanGreetings = []string{
	"alo",
	"ola",
	"oi",
	"sim",
	"pronto",
	"pois nao",
	"quem fala",
	"quem e",
	"quem ta falando",
	"com quem",
	"fala",
	"opa",
	"bom dia",
	"boa tarde",
	"boa noite",
	"pode falar",
	"aqui e",
	"sou eu",
	"e ele",
	"e ela",
	"ele mesmo",
	"ela mesma",
	"eu mesmo",
	"com ele",
	"com ela",
	"o que deseja",
	"diga",
	"tudo bem",
	"tudo bom",
	"beleza",
	"joia",
	"estou ouvindo",
	"to ouvindo",
	"na escuta",
	"pode dizer",
}

func loadDynamicConfig(maxDuration *float64) {
	for _, p := range []string{"/etc/asterisk/vosk_amd.json", "/opt/ominichat/asterisk/conf/vosk_amd.json", "./storage/vosk_amd.json"} {
		if data, err := os.ReadFile(p); err == nil {
			var cfg struct {
				VoskMaxDurationSec float64  `json:"vosk_max_duration_sec"`
				VoicemailPhrases   []string `json:"voicemail_phrases"`
				HumanGreetings     []string `json:"human_greetings"`
			}
			if json.Unmarshal(data, &cfg) == nil {
				if cfg.VoskMaxDurationSec > 0 {
					*maxDuration = cfg.VoskMaxDurationSec
				}
				if len(cfg.VoicemailPhrases) > 0 {
					voicemailPhrases = cfg.VoicemailPhrases
				}
				if len(cfg.HumanGreetings) > 0 {
					humanGreetings = cfg.HumanGreetings
				}
				break
			}
		}
	}
}

func normalizeText(s string) string {
	if s == "" {
		return ""
	}
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	res, _, err := transform.String(t, s)
	if err != nil {
		res = s
	}
	var sb strings.Builder
	for _, r := range strings.ToLower(res) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			sb.WriteRune(r)
		} else {
			sb.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(sb.String()), " ")
}

// matchWordOrPhrase verifica se a palavra ou frase ocorre com fronteiras limpas no texto.
// Evita falsos positivos como "oito" casar com "oi" ou "assim" casar com "sim".
func matchWordOrPhrase(text, phrase string) bool {
	normText := normalizeText(text)
	normPhrase := normalizeText(phrase)
	if normText == "" || normPhrase == "" {
		return false
	}
	paddedText := " " + normText + " "
	paddedPhrase := " " + normPhrase + " "
	return strings.Contains(paddedText, paddedPhrase)
}

// ClassifyOutcome analisa o texto acumulado da chamada e aplica AVALIAÇÃO POSITIVA DE ATENDIMENTO.
// Regra de Negócio: Só é classificado como HUMAN se houver confirmação positiva inequívoca (ex: 'alô', 'sim', 'pronto').
// Silêncio, ruídos de linha, caixas postais ou áudios ininteligíveis são classificados como MACHINE para evitar abandono de operadores.
func ClassifyOutcome(fullText string, vmPhrases, greetings []string) (status, cause string) {
	if len(vmPhrases) == 0 {
		vmPhrases = voicemailPhrases
	}
	if len(greetings) == 0 {
		greetings = humanGreetings
	}

	norm := normalizeText(fullText)
	if norm == "" {
		// Cliente permaneceu em silêncio após a saudação: não repassa ao operador
		return "MACHINE", "SILENCE_TIMEOUT"
	}

	// 1. Checagem prioritária de termos de caixa postal / correio de voz / mensagem de operadora
	for _, phrase := range vmPhrases {
		if matchWordOrPhrase(norm, phrase) {
			return "MACHINE", "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_"))
		}
	}

	// 2. Checagem estritamente positiva de saudações / confirmações humanas
	for _, greeting := range greetings {
		if matchWordOrPhrase(norm, greeting) {
			return "HUMAN", "HUMAN_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_"))
		}
	}

	// 3. Texto falado que não corresponde a nenhuma saudação positiva (ex: ruído de fundo, TV, áudio ininteligível)
	return "MACHINE", "UNCONFIRMED_AUDIO"
}

