package main

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

type CallMetrics struct {
	FullText          string  // Transcrição acumulada do Vosk
	SpeechDurationSec float64 // Tempo de fala acumulado
	SilenceAfterSec   float64 // Tempo de silêncio após a última fala
}

var (
	configMu             sync.RWMutex
	highConfidenceVM     []string
	quickHumanGreetings  []string
	normalizedVMList     []string
	normalizedGreetList  []string
)

func init() {
	highConfidenceVM = []string{
		"caixa postal",
		"deixe seu recado",
		"grave seu recado",
		"deixe sua mensagem",
		"apos o sinal",
		"apos o bip",
		"secretaria eletronica",
		"mensagem gravada",
		"horario de atendimento",
		"vivo informa",
		"claro informa",
		"tim informa",
		"nao pode receber chamadas",
		"temporariamente fora de servico",
		"este numero de telefone nao",
		"o numero discado nao existe",
		"programado para nao receber",
		"caixa de mensagens",
	}

	quickHumanGreetings = []string{
		"alo", "a lo", "ola", "oi", "pronto", "pois nao",
		"quem fala", "quem e", "quem ta falando", "com quem falo",
		"fala", "fala ai", "opa", "bom dia", "boa tarde", "boa noite",
		"pode falar", "diga", "diz", "tudo bem", "tudo bom",
		"beleza", "joia", "estou ouvindo", "to ouvindo", "na escuta",
	}

	precomputeNormalizedLists()
}

func precomputeNormalizedLists() {
	normalizedVMList = make([]string, len(highConfidenceVM))
	for i, phrase := range highConfidenceVM {
		normalizedVMList[i] = normalizeText(phrase)
	}

	normalizedGreetList = make([]string, len(quickHumanGreetings))
	for i, greet := range quickHumanGreetings {
		normalizedGreetList[i] = normalizeText(greet)
	}
}

func LoadDynamicConfig(maxDuration *float64) {
	paths := []string{
		"/etc/asterisk/vosk_amd.json",
		"/opt/ominichat/asterisk/conf/vosk_amd.json",
		"./storage/vosk_amd.json",
	}

	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}

		var cfg struct {
			VoskMaxDurationSec float64  `json:"vosk_max_duration_sec"`
			VoicemailPhrases   []string `json:"voicemail_phrases"`
			HumanGreetings     []string `json:"human_greetings"`
		}

		if err := json.Unmarshal(data, &cfg); err == nil {
			configMu.Lock()
			if cfg.VoskMaxDurationSec > 0 && maxDuration != nil {
				*maxDuration = cfg.VoskMaxDurationSec
			}
			if len(cfg.VoicemailPhrases) > 0 {
				highConfidenceVM = cfg.VoicemailPhrases
			}
			if len(cfg.HumanGreetings) > 0 {
				quickHumanGreetings = cfg.HumanGreetings
			}
			precomputeNormalizedLists()
			configMu.Unlock()
			break
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

// Levenshtein otimizado para reaproveitar memória em apenas 1 vetor
func levenshtein(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	la, lb := len(ra), len(rb)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	if la > lb {
		ra, rb = rb, ra
		la, lb = lb, la
	}

	row := make([]int, la+1)
	for i := 0; i <= la; i++ {
		row[i] = i
	}

	for j := 1; j <= lb; j++ {
		prev := row[0]
		row[0] = j
		for i := 1; i <= la; i++ {
			cost := 0
			if ra[i-1] != rb[j-1] {
				cost = 1
			}
			temp := row[i]
			minVal := row[i] + 1
			if row[i-1]+1 < minVal {
				minVal = row[i-1] + 1
			}
			if prev+cost < minVal {
				minVal = prev + cost
			}
			row[i] = minVal
			prev = temp
		}
	}
	return row[la]
}

func fuzzyContainsPhrase(textWords []string, targetWords []string, maxDistance int) bool {
	if len(targetWords) == 0 || len(textWords) < len(targetWords) {
		return false
	}

	targetJoined := strings.Join(targetWords, " ")
	windowSize := len(targetWords)

	for i := 0; i <= len(textWords)-windowSize; i++ {
		window := strings.Join(textWords[i:i+windowSize], " ")
		if window == targetJoined {
			return true
		}
		if levenshtein(window, targetJoined) <= maxDistance {
			return true
		}
	}
	return false
}

func ClassifyCall(metrics CallMetrics) (status, reason string) {
	normText := normalizeText(metrics.FullText)
	words := strings.Fields(normText)
	wordCount := len(words)

	configMu.RLock()
	vmPhrases := normalizedVMList
	greetings := normalizedGreetList
	configMu.RUnlock()

	paddedText := " " + normText + " "

	// 1. CHECAGEM DE SAUDAÇÃO HUMANA PRIORITÁRIA
	// Se começou com saudação humana e não se estendeu em discurso longo, é humano
	hasGreeting := false
	for _, greeting := range greetings {
		if strings.Contains(paddedText, " "+greeting+" ") {
			hasGreeting = true
			break
		}
	}

	if hasGreeting {
		// Humano falando naturalmente (ex: "Alô, bom dia! Quem fala por gentileza?")
		if wordCount <= 8 && metrics.SpeechDurationSec <= 3.5 {
			return "HUMAN", "HUMAN_GREETING_CONFIRMED"
		}
		// Se disse alô e calou a boca por mais de 400ms, transfere imediatamente
		if metrics.SilenceAfterSec >= 0.4 && wordCount <= 10 {
			return "HUMAN", "HUMAN_GREETING_AND_PAUSE"
		}
	}

	// 2. DETECÇÃO DE CAIXA POSTAL / URA DE OPERADORA
	for _, normPhrase := range vmPhrases {
		targetWords := strings.Fields(normPhrase)
		maxTolerance := 1
		if len(targetWords) > 2 {
			maxTolerance = 2
		}

		if fuzzyContainsPhrase(words, targetWords, maxTolerance) {
			// Salvaguarda: se tiver dito "alô" no início, só classifica como máquina
			// se a frase de caixa tiver vindo explicitamente com volume alto de palavras
			if hasGreeting && wordCount <= 4 {
				return "HUMAN", "HUMAN_GREETING_OVERRIDE"
			}
			return "MACHINE", "VOICEMAIL_MATCH_" + strings.ToUpper(strings.ReplaceAll(normPhrase, " ", "_"))
		}
	}

	// 3. ANÁLISE DE MONÓLOGO CONTÍNUO (URAs institucionais / secretárias longas)
	// Só aciona se a fala for longa, sem nenhuma pausa relevante e SEM saudação de resposta curta
	if metrics.SpeechDurationSec >= 4.2 && wordCount >= 12 && metrics.SilenceAfterSec < 0.6 && !hasGreeting {
		return "MACHINE", "CONTINUOUS_MONOLOGUE_DETECTED"
	}

	// 4. CLIENTE COM RESPOSTA CURTA OU EM SILÊNCIO APÓS ATENDER
	if wordCount <= 3 {
		return "HUMAN", "HUMAN_NATURAL_PAUSE"
	}

	// 5. REGRA DE SEGURANÇA (Dúvida sempre entrega para o operador)
	return "HUMAN", "FALLBACK_ASSUMED_HUMAN"
}

func ClassifyOutcome(fullText string, vmPhrases, greetings []string) (status, cause string) {
	return ClassifyCall(CallMetrics{
		FullText: fullText,
	})
}
