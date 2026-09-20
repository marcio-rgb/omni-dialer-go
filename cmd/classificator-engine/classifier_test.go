package main

import (
	"testing"

	"dialer-go/internal/domain"
)

func TestSemanticClassifier_Benchmark30Audios(t *testing.T) {
	classifier := NewSemanticClassifier()

	tests := []struct {
		id       int
		name     string
		input    string
		expected domain.ClassificationStatus
	}{
		{1, "sim_01_human_alo", "Alô", domain.StatusHuman},
		{2, "sim_02_human_alo_boa_noite", "Alô, boa noite", domain.StatusHuman},
		{3, "sim_03_human_alo_boa_tarde", "Alô, boa tarde", domain.StatusHuman},
		{4, "sim_04_human_alo_bom_dia", "Alô, bom dia", domain.StatusHuman},
		{5, "sim_05_human_alo_com_quem_falo", "Alô, com quem eu falo?", domain.StatusHuman},
		{6, "sim_06_human_alo_nao_estou_ouvindo", "Alô, não estou te ouvindo bem", domain.StatusHuman},
		{7, "sim_07_human_alo_quem_chamando", "Alô, quem está chamando?", domain.StatusHuman},
		{8, "sim_08_human_alo_quem_fala", "Alô, quem fala?", domain.StatusHuman},
		{9, "sim_09_human_alo_so_um_momento", "Alô, só um momento por favor", domain.StatusHuman},
		{10, "sim_10_machine_caixa_postal_deixe_recado", "Caixa postal. Deixe seu recado após o sinal.", domain.StatusMachine},
		{11, "sim_11_machine_chamada_encaminhada_caixa", "Chamada encaminhada para a caixa postal", domain.StatusMachine},
		{12, "sim_12_machine_caixa_de_mensagens", "Esta é a caixa de mensagens, deixe seu recado", domain.StatusMachine},
		{13, "sim_13_human_fala_pode_falar", "Fala, pode falar", domain.StatusHuman},
		{14, "sim_14_machine_nao_foi_possivel_completar", "Não foi possível completar sua chamada, verifique o número", domain.StatusMachine},
		{15, "sim_15_machine_todos_nossos_atendentes", "No momento todos os nossos atendentes estão ocupados", domain.StatusMachine},
		{16, "sim_16_machine_numero_nao_pode_receber", "O número chamado não pode receber chamadas neste momento", domain.StatusMachine},
		{17, "sim_17_machine_celular_encontra_se", "O telefone celular chamado encontra-se desligado", domain.StatusMachine},
		{18, "sim_18_human_oi_marcio_quem_fala", "Oi, é o Márcio, quem fala?", domain.StatusHuman},
		{19, "sim_19_human_oi_boa_tarde_quem_e", "Oi, boa tarde, quem é?", domain.StatusHuman},
		{20, "sim_20_human_oi_pois_nao", "Oi, pois não", domain.StatusHuman},
		{21, "sim_21_human_oi_tudo_bem_quem_ta_falando", "Oi, tudo bem? Quem tá falando?", domain.StatusHuman},
		{22, "sim_22_machine_nao_posso_atender_agora_1", "Olá, não posso atender agora, deixe seu recado após o bip", domain.StatusMachine},
		{23, "sim_23_machine_nao_posso_atender_agora_2", "Olá! Não posso atender agora, deixe sua mensagem.", domain.StatusMachine},
		{24, "sim_24_human_opa_tudo_bem_1", "Opa, e aí, tudo bem?", domain.StatusHuman},
		{25, "sim_25_human_opa_tudo_bem_2", "Opa, e aí, tudo bem?", domain.StatusHuman},
		{26, "sim_26_human_pois_nao_posso_ajudar", "Pois não, em que posso ajudar?", domain.StatusHuman},
		{27, "sim_27_human_pronto_com_quem_falo", "Pronto, com quem eu falo por favor?", domain.StatusHuman},
		{28, "sim_28_human_pronto_pode_falar", "Pronto, pode falar", domain.StatusHuman},
		{29, "sim_29_human_sim_quem_gostaria", "Sim, quem gostaria?", domain.StatusHuman},
		{30, "sim_30_machine_chamada_sendo_encaminhada", "Sua chamada está sendo encaminhada para a caixa postal", domain.StatusMachine},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 1. Testa Fast-Exit (Evaluate)
			verdict, isFinal := classifier.Evaluate(tt.input, "", 1.0, 0.2)
			if !isFinal {
				// Se não foi Fast-Exit, testa Finalize
				verdict = classifier.Finalize(tt.input, 1.5, 0.3)
			}

			if verdict.Status != tt.expected {
				t.Errorf("[FALHA] Audio #%02d (%s): esperado %s, obtido %s (Causa: %s | Texto: '%s')",
					tt.id, tt.name, tt.expected, verdict.Status, verdict.Cause, tt.input)
			} else {
				t.Logf("[OK] Audio #%02d (%s): %s (Causa: %s)", tt.id, tt.name, verdict.Status, verdict.Cause)
			}
		})
	}
}

func TestSemanticClassifier_VoicemailPrecedenceOverGreeting(t *testing.T) {
	classifier := NewSemanticClassifier()

	// Casos críticos onde a secretária começa com "Olá" ou "Oi" mas é caixa postal
	trickyCases := []string{
		"Olá, não posso atender agora",
		"Oi, você ligou para fulano mas no momento não posso atender",
		"Olá, deixe seu recado após o sinal",
		"Bom dia, no momento todos os nossos atendentes estão ocupados",
		"Alô, sua chamada está sendo encaminhada para a caixa postal",
	}

	for _, text := range trickyCases {
		verdict, isFinal := classifier.Evaluate(text, "", 1.0, 0.1)
		if !isFinal {
			verdict = classifier.Finalize(text, 1.5, 0.2)
		}
		if verdict.Status != domain.StatusMachine {
			t.Errorf("[FALHA CRITICA] Texto '%s' deveria ser MACHINE, mas retornou %s (Causa: %s)",
				text, verdict.Status, verdict.Cause)
		}
	}
}
