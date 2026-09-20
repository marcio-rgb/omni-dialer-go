package main

import (
	"strings"
	"sync"
	"time"
	"unicode"

	"dialer-go/internal/domain"
	"dialer-go/internal/ports"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

// SemanticClassifier implementa a interface ports.SemanticClassifierPort.
//
// @pattern Strategy (Canonical Classifier)
// @governedBy .agents/ARCHITECT.md
type SemanticClassifier struct {
	mu                  sync.RWMutex
	highConfidenceVM    []string
	quickHumanGreetings []string
	normalizedVMList    []string
	normalizedGreetList []string
}

// NewSemanticClassifier cria e inicializa os dicionários semânticos pré-compilados.
func NewSemanticClassifier() ports.SemanticClassifierPort {
	c := &SemanticClassifier{
		highConfidenceVM: []string{
			// Caixas Postais e Recados
			"caixa postal", "deixe seu recado", "grave seu recado", "deixe sua mensagem",
			"apos o sinal", "apos o bip", "secretaria eletronica", "mensagem gravada",
			"caixa de mensagens", "esta e a caixa de mensagens", "caixa de mensagem",
			// Indisponibilidade e Recusa de Atendimento
			"nao posso atender agora", "nao posso atender", "nao podemos atender",
			"nao pode atender no momento", "impossibilitado de atender",
			// Atendentes Ocupados / URAs
			"todos os nossos atendentes", "nossos atendentes estao ocupados",
			"todos os atendentes estao ocupados", "atendentes ocupados",
			"horario de atendimento", "para falar com",
			// Encaminhamento de Chamadas
			"chamada encaminhada", "chamada sendo encaminhada", "sendo encaminhada para",
			"sendo encaminhada para a caixa", "encaminhada para a caixa postal",
			// Mensagens de Erro de Operadora
			"nao foi possivel completar", "nao foi possivel completar sua chamada",
			"impossivel completar sua chamada", "nao pode receber chamadas",
			"o numero chamado nao pode receber", "nao pode receber ligacoes",
			"encontra se desligado", "encontra se fora", "fora da area de cobertura",
			"telefone celular chamado encontra se", "celular chamado encontra se",
			"temporariamente fora de servico", "este numero de telefone nao",
			"o numero discado nao existe", "programado para nao receber",
			"vivo informa", "claro informa", "tim informa", "oi informa",
		},
		quickHumanGreetings: []string{
			"alo", "a lo", "ola", "oi", "pronto", "pois nao",
			"quem fala", "quem e", "quem ta falando", "com quem falo", "com quem eu falo",
			"fala", "fala ai", "opa", "bom dia", "boa tarde", "boa noite",
			"pode falar", "diga", "diz", "tudo bem", "tudo bom",
			"beleza", "joia", "estou ouvindo", "to ouvindo", "na escuta",
			"sim quem gostaria", "quem gostaria", "em que posso ajudar",
		},
	}
	c.precomputeNormalizedLists()
	return c
}

func (c *SemanticClassifier) precomputeNormalizedLists() {
	c.normalizedVMList = make([]string, len(c.highConfidenceVM))
	for i, phrase := range c.highConfidenceVM {
		c.normalizedVMList[i] = normalizeText(phrase)
	}

	c.normalizedGreetList = make([]string, len(c.quickHumanGreetings))
	for i, greet := range c.quickHumanGreetings {
		c.normalizedGreetList[i] = normalizeText(greet)
	}
}

// Evaluate avalia o texto corrente em tempo real para Fast-Exit.
//
// @pattern Fast-Exit Evaluation
// @preExecution Textos parciais ou finais normalizados.
// @postExecution Retorna o veredito canônico estruturado.
func (c *SemanticClassifier) Evaluate(partialText, finalText string, speechSec, silenceSec float64) (domain.ClassificationVerdict, bool) {
	current := strings.TrimSpace(finalText)
	if current == "" {
		current = strings.TrimSpace(partialText)
	}
	if current == "" {
		return domain.ClassificationVerdict{}, false
	}

	normCurrent := normalizeText(current)
	normFinal := normalizeText(finalText)
	paddedCurrent := " " + normCurrent + " "
	paddedFinal := " " + normFinal + " "

	c.mu.RLock()
	vmPhrases := c.normalizedVMList
	greetings := c.normalizedGreetList
	c.mu.RUnlock()

	currWords := strings.Fields(normCurrent)
	finalWords := strings.Fields(normFinal)

	// 1. DETECÇÃO PRIORITÁRIA DE CAIXA POSTAL / URA COM TOLERÂNCIA RESTRITA
	// Regra de Ouro: Mensagens como "Olá, não posso atender agora..." DEVEM ser detectadas como MACHINE!
	for _, phrase := range vmPhrases {
		targetWords := strings.Fields(phrase)
		maxTolerance := 0
		if len(targetWords) >= 3 {
			maxTolerance = 1
		}

		if fuzzyContainsPhrase(currWords, targetWords, maxTolerance) || fuzzyContainsPhrase(finalWords, targetWords, maxTolerance) {
			return domain.ClassificationVerdict{
				Status:        domain.StatusMachine,
				Cause:         "VOICEMAIL_MATCH_" + strings.ToUpper(strings.ReplaceAll(phrase, " ", "_")),
				Transcription: current,
				Confidence:    0.98,
				SpeechSec:     speechSec,
				SilenceSec:    silenceSec,
				CreatedAt:     time.Now(),
			}, true
		}
	}

	// 2. CHECAGEM DE SAUDAÇÃO HUMANA RÁPIDA (SOMENTE SE NÃO FOR CAIXA POSTAL)
	for _, greeting := range greetings {
		paddedGreeting := " " + greeting + " "
		if strings.Contains(paddedCurrent, paddedGreeting) || strings.Contains(paddedFinal, paddedGreeting) {
			return domain.ClassificationVerdict{
				Status:        domain.StatusHuman,
				Cause:         "HUMAN_GREETING_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_")),
				Transcription: current,
				Confidence:    0.99,
				SpeechSec:     speechSec,
				SilenceSec:    silenceSec,
				CreatedAt:     time.Now(),
			}, true
		}
	}

	return domain.ClassificationVerdict{}, false
}

// Finalize emite a classificação final ao término do fluxo de áudio da chamada.
func (c *SemanticClassifier) Finalize(finalText string, speechSec, silenceSec float64) domain.ClassificationVerdict {
	normText := normalizeText(finalText)
	words := strings.Fields(normText)
	wordCount := len(words)

	c.mu.RLock()
	vmPhrases := c.normalizedVMList
	greetings := c.normalizedGreetList
	c.mu.RUnlock()

	paddedText := " " + normText + " "

	// 1. Checagem Prioritária de Caixa Postal / URA
	for _, normPhrase := range vmPhrases {
		targetWords := strings.Fields(normPhrase)
		maxTolerance := 0
		if len(targetWords) >= 3 {
			maxTolerance = 1
		}
		if fuzzyContainsPhrase(words, targetWords, maxTolerance) {
			return domain.ClassificationVerdict{
				Status:        domain.StatusMachine,
				Cause:         "VOICEMAIL_MATCH_" + strings.ToUpper(strings.ReplaceAll(normPhrase, " ", "_")),
				Transcription: finalText,
				Confidence:    0.97,
				SpeechSec:     speechSec,
				SilenceSec:    silenceSec,
				CreatedAt:     time.Now(),
			}
		}
	}

	// 2. Checagem de Saudação Humana
	for _, greeting := range greetings {
		if strings.Contains(paddedText, " "+greeting+" ") {
			return domain.ClassificationVerdict{
				Status:        domain.StatusHuman,
				Cause:         "HUMAN_GREETING_" + strings.ToUpper(strings.ReplaceAll(greeting, " ", "_")),
				Transcription: finalText,
				Confidence:    0.98,
				SpeechSec:     speechSec,
				SilenceSec:    silenceSec,
				CreatedAt:     time.Now(),
			}
		}
	}

	// 3. Monólogo Contínuo (URA institucional longa sem pausa)
	if speechSec >= 2.8 && wordCount >= 8 && silenceSec < 0.6 {
		return domain.ClassificationVerdict{
			Status:        domain.StatusMachine,
			Cause:         "CONTINUOUS_MONOLOGUE_DETECTED",
			Transcription: finalText,
			Confidence:    0.92,
			SpeechSec:     speechSec,
			SilenceSec:    silenceSec,
			CreatedAt:     time.Now(),
		}
	}

	// 4. Silêncio após atendimento humano (Regra de Ouro: HUMANO)
	if wordCount <= 3 {
		return domain.ClassificationVerdict{
			Status:        domain.StatusHuman,
			Cause:         "HUMAN_NATURAL_PAUSE",
			Transcription: finalText,
			Confidence:    0.95,
			SpeechSec:     speechSec,
			SilenceSec:    silenceSec,
			CreatedAt:     time.Now(),
		}
	}

	// 5. Fallback seguro geral
	return domain.ClassificationVerdict{
		Status:        domain.StatusHuman,
		Cause:         "FALLBACK_ASSUMED_HUMAN",
		Transcription: finalText,
		Confidence:    0.90,
		SpeechSec:     speechSec,
		SilenceSec:    silenceSec,
		CreatedAt:     time.Now(),
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
