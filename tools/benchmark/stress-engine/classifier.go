package main

import (
	"strings"
	"sync"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

/**
 * Classificador semântico de fala (Ground Truth: HUMAN vs MACHINE).
 *
 * @pattern Strategy / Rules Engine
 * @governedBy .agents/workflows/agente-especialista-sst.md
 */

type CallMetrics struct {
	FullText          string
	SpeechDurationSec float64
	SilenceAfterSec   float64
}

var (
	configMu            sync.RWMutex
	highConfidenceVM    []string
	quickHumanGreetings []string
	normalizedVMList    []string
	normalizedGreetList []string
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
		"oi informa",
		"nao pode receber chamadas",
		"temporariamente fora de servico",
		"este numero de telefone nao",
		"o numero discado nao existe",
		"programado para nao receber",
		"caixa de mensagens",
		"sua chamada foi",
		"sua ligacao",
		"ligacao foi",
		"chamada foi",
		"correio de voz",
		"recado apos",
		"recado",
		"nao pode atender",
		"encaminhada para a caixa",
		"encaminhado para",
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

func normalizeText(s string) string {
	t := transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)
	result, _, _ := transform.String(t, s)
	result = strings.ToLower(result)

	var b strings.Builder
	for _, r := range result {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

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
		if maxDistance > 0 && levenshtein(window, targetJoined) <= maxDistance {
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

	// 1. Saudação Humana Prioritária (Regra de Ouro)
	hasGreeting := false
	matchedGreeting := ""
	for _, greeting := range greetings {
		if strings.Contains(paddedText, " "+greeting+" ") {
			hasGreeting = true
			matchedGreeting = greeting
			break
		}
	}

	if hasGreeting {
		hasVMConflict := false
		conflictVM := ""
		for _, vm := range vmPhrases {
			vmWords := strings.Fields(vm)
			dist := 0
			if len(vmWords) >= 3 {
				dist = 1
			}
			if fuzzyContainsPhrase(words, vmWords, dist) {
				hasVMConflict = true
				conflictVM = vm
				break
			}
		}

		if hasVMConflict {
			return "MACHINE", "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(conflictVM, " ", "_"))
		}
		return "HUMAN", "HUMAN_GREETING_" + strings.ToUpper(strings.ReplaceAll(matchedGreeting, " ", "_"))
	}

	// 2. Caixa Postal Sem Saudação
	for _, vm := range vmPhrases {
		vmWords := strings.Fields(vm)
		dist := 0
		if len(vmWords) >= 3 {
			dist = 1
		}
		if fuzzyContainsPhrase(words, vmWords, dist) {
			return "MACHINE", "VOICEMAIL_" + strings.ToUpper(strings.ReplaceAll(vm, " ", "_"))
		}
	}

	// 3. Critérios de Fala Natural vs Silêncio
	if wordCount == 0 {
		return "HUMAN", "HUMAN_SILENCE_ASSUMED"
	}
	if wordCount >= 1 && wordCount <= 5 {
		return "HUMAN", "HUMAN_SHORT_SPEECH"
	}

	return "HUMAN", "HUMAN_NATURAL_SPEECH"
}
