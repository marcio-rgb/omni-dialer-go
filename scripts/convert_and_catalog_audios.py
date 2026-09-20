#!/usr/bin/env python3
import os
import subprocess
import json

# Mapeamento canônico dos 30 áudios
AUDIO_CATALOG = [
    {"id": 1, "src": "Al__.mp3", "name": "sim_01_human_alo", "type": "HUMAN", "desc": "Alô"},
    {"id": 2, "src": "Al___boa_noite_.mp3", "name": "sim_02_human_alo_boa_noite", "type": "HUMAN", "desc": "Alô, boa noite"},
    {"id": 3, "src": "Al___boa_tarde_.mp3", "name": "sim_03_human_alo_boa_tarde", "type": "HUMAN", "desc": "Alô, boa tarde"},
    {"id": 4, "src": "Al___bom_dia_.mp3", "name": "sim_04_human_alo_bom_dia", "type": "HUMAN", "desc": "Alô, bom dia"},
    {"id": 5, "src": "Al___com_quem_eu_falo_.mp3", "name": "sim_05_human_alo_com_quem_falo", "type": "HUMAN", "desc": "Alô, com quem eu falo?"},
    {"id": 6, "src": "Al___n_o_estou_te_ouvindo_bem_.mp3", "name": "sim_06_human_alo_nao_estou_ouvindo", "type": "HUMAN", "desc": "Alô, não estou te ouvindo bem"},
    {"id": 7, "src": "Al___quem_est__chamando_.mp3", "name": "sim_07_human_alo_quem_chamando", "type": "HUMAN", "desc": "Alô, quem está chamando?"},
    {"id": 8, "src": "Al___quem_fala_.mp3", "name": "sim_08_human_alo_quem_fala", "type": "HUMAN", "desc": "Alô, quem fala?"},
    {"id": 9, "src": "Al___s__um_momento__por_favor_.mp3", "name": "sim_09_human_alo_so_um_momento", "type": "HUMAN", "desc": "Alô, só um momento por favor"},
    {"id": 10, "src": "Caixa_postal__Deixe_seu_recado.mp3", "name": "sim_10_machine_caixa_postal_deixe_recado", "type": "MACHINE", "desc": "Caixa postal. Deixe seu recado..."},
    {"id": 11, "src": "Chamada_encaminhada_para_a_cai.mp3", "name": "sim_11_machine_chamada_encaminhada_caixa", "type": "MACHINE", "desc": "Chamada encaminhada para a caixa postal..."},
    {"id": 12, "src": "Esta___a_caixa_de_mensagens__P.mp3", "name": "sim_12_machine_caixa_de_mensagens", "type": "MACHINE", "desc": "Esta é a caixa de mensagens..."},
    {"id": 13, "src": "Fala__pode_falar_.mp3", "name": "sim_13_human_fala_pode_falar", "type": "HUMAN", "desc": "Fala, pode falar"},
    {"id": 14, "src": "N_o_foi_poss_vel_completar_sua (2).mp3", "name": "sim_14_machine_nao_foi_possivel_completar", "type": "MACHINE", "desc": "Não foi possível completar sua chamada..."},
    {"id": 15, "src": "No_momento_todos_os_nossos_ate (1).mp3", "name": "sim_15_machine_todos_nossos_atendentes", "type": "MACHINE", "desc": "No momento todos os nossos atendentes..."},
    {"id": 16, "src": "O_n_mero_chamado_n_o_pode_rece (2).mp3", "name": "sim_16_machine_numero_nao_pode_receber", "type": "MACHINE", "desc": "O número chamado não pode receber chamadas..."},
    {"id": 17, "src": "O_telefone_celular_chamado_enc (2).mp3", "name": "sim_17_machine_celular_encontra_se", "type": "MACHINE", "desc": "O telefone celular chamado encontra-se desligado..."},
    {"id": 18, "src": "Oi____o_M_rcio__quem_fala_.mp3", "name": "sim_18_human_oi_marcio_quem_fala", "type": "HUMAN", "desc": "Oi, é o Márcio, quem fala?"},
    {"id": 19, "src": "Oi__boa_tarde__quem___.mp3", "name": "sim_19_human_oi_boa_tarde_quem_e", "type": "HUMAN", "desc": "Oi, boa tarde, quem é?"},
    {"id": 20, "src": "Oi__pois_n_o_.mp3", "name": "sim_20_human_oi_pois_nao", "type": "HUMAN", "desc": "Oi, pois não"},
    {"id": 21, "src": "Oi__tudo_bem__Quem_t__falando_.mp3", "name": "sim_21_human_oi_tudo_bem_quem_ta_falando", "type": "HUMAN", "desc": "Oi, tudo bem? Quem tá falando?"},
    {"id": 22, "src": "Ol___n_o_posso_atender_agora__ (1).mp3", "name": "sim_22_machine_nao_posso_atender_agora_1", "type": "MACHINE", "desc": "Olá, não posso atender agora..."},
    {"id": 23, "src": "Ol___n_o_posso_atender_agora__ (2).mp3", "name": "sim_23_machine_nao_posso_atender_agora_2", "type": "MACHINE", "desc": "Olá, não posso atender agora..."},
    {"id": 24, "src": "Opa__e_a___tudo_bem_ (1).mp3", "name": "sim_24_human_opa_tudo_bem_1", "type": "HUMAN", "desc": "Opa, e aí, tudo bem?"},
    {"id": 25, "src": "Opa__e_a___tudo_bem_.mp3", "name": "sim_25_human_opa_tudo_bem_2", "type": "HUMAN", "desc": "Opa, e aí, tudo bem?"},
    {"id": 26, "src": "Pois_n_o__em_que_posso_ajudar_.mp3", "name": "sim_26_human_pois_nao_posso_ajudar", "type": "HUMAN", "desc": "Pois não, em que posso ajudar?"},
    {"id": 27, "src": "Pronto__com_quem_eu_falo__por_.mp3", "name": "sim_27_human_pronto_com_quem_falo", "type": "HUMAN", "desc": "Pronto, com quem eu falo por favor?"},
    {"id": 28, "src": "Pronto__pode_falar_.mp3", "name": "sim_28_human_pronto_pode_falar", "type": "HUMAN", "desc": "Pronto, pode falar"},
    {"id": 29, "src": "Sim__quem_gostaria_.mp3", "name": "sim_29_human_sim_quem_gostaria", "type": "HUMAN", "desc": "Sim, quem gostaria?"},
    {"id": 30, "src": "Sua_chamada_est__sendo_encamin (2).mp3", "name": "sim_30_machine_chamada_sendo_encaminhada", "type": "MACHINE", "desc": "Sua chamada está sendo encaminhada..."}
]

def main():
    src_dir = "/opt/ominichat/classificator/raw_mp3"
    out_dir = "/opt/ominichat/classificator/benchmark_wav"
    os.makedirs(out_dir, exist_ok=True)
    
    print(f">> Convertendo {len(AUDIO_CATALOG)} arquivos para WAV PCM linear 16-bit 8000 Hz Mono...")
    
    for item in AUDIO_CATALOG:
        in_path = os.path.join(src_dir, item["src"])
        out_path = os.path.join(out_dir, f"{item['name']}.wav")
        
        cmd = [
            "ffmpeg", "-y", "-i", in_path,
            "-ar", "8000", "-ac", "1", "-c:a", "pcm_s16le",
            out_path
        ]
        res = subprocess.run(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if res.returncode != 0:
            print(f"❌ Falha ao converter {item['src']}")
        else:
            print(f"✅ [{item['id']:02d}] {item['type']:7s} -> {item['name']}.wav")
            
    # Salva catálogo JSON para consumo do gerador de dialplan e auditoria
    catalog_json_path = os.path.join(out_dir, "catalog.json")
    with open(catalog_json_path, "w", encoding="utf-8") as f:
        json.dump(AUDIO_CATALOG, f, indent=2, ensure_ascii=False)
    print(f"\n>> Catálogo salvo com sucesso em: {catalog_json_path}")

if __name__ == "__main__":
    main()
