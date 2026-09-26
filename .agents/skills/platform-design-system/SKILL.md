***

## name: platform-design-system description: Diretrizes de design system, paleta Warm Dark Mode, tipografia, ícones PrimeIcons e regras de UI para a plataforma CBR Estúdio IA.

# Skill de Design System: dialer-go

Esta skill padroniza os princípios visuais, estruturais e comportamentais da interface do CBR Estúdio IA.

## 1. Identidade Visual e Paleta de Cores (Warm Dark Mode)

* **Fundo Principal (Telas):** `bg-dark-900` / Tailwind `stone-900` (`#141211`).

* **Painéis / Cards:** `bg-dark-800` / Tailwind `stone-800` (`#1c1917`).

* **Bordas e Divisórias:** `border-stone-800` a `border-stone-700/60`.

* **Destaques (Accent Gradient):** `bg-gradient-to-r from-orange-700 to-amber-600` (Hover: `from-orange-600 to-amber-500`).

* **Texto Principal:** `text-stone-100` (`#f5f5f4`) - É proibido branco puro `#ffffff` desprovido de temperatura.

* **Texto Secundário / Labels:** `text-stone-400` (`#a8a29e`).

## 2. Tipografia Tripla

* **Interface Geral:** Font `Inter` (`font-sans`), tamanhos `text-xs` a `text-sm`.

* **Títulos e Painéis:** Font `Outfit` (`font-display`), peso `font-semibold`.

* **Código, Prompts e Terminal:** Font `Fira Code` (`font-mono`), fundo `bg-dark-900`.

## 3. Ícones Neutros e Emojis

* **Biblioteca Obrigatória:** PrimeIcons (`pi pi-*`).

* **Paleta de Ícones (Neutros e Sóbrios):** Damos preferência absoluta para **ícones neutros e monocromáticos** em tons de cinza-claro e branco (`text-stone-300`, `text-stone-400`, `text-stone-200`). Evitar o uso excessivo de cores (verde neon, amarelo berrante ou vermelho saturado), reservando cores apenas para alertas críticos.

* **Emojis:** Estritamente proibidos em qualquer componente de interface.

## 4. Botões Somente com Ícone (Icon-Only) e Botões sem Borda (Ghost Buttons)

* **Preferência por Botões sem Texto (Icon-Only):** Para ações secundárias, utilitários, barras de ferramentas e controles (ex: *Parar*, *Copiar*, *Download*, *Filtro*, *Configurações*, *Limpar*, *Alternar*, *Fechar*), dar preferência estrita a botões compactos contendo **apenas o ícone** acompanhado do atributo `title` (tooltip descritivo).

* **Botões Compactos sem Borda (Ghost Icon Buttons) em Cards e Linhas:** Para ações rápidas em linha dentro de cartões, listas e passos (ex: *Play do passo*, *Refatorar IA*, *Mais opções*), utilizar botões minimalistas **sem bordas** com hover sutil:

```html
<button class="w-6 h-6 rounded flex items-center justify-center text-stone-400 hover:text-stone-100 hover:bg-stone-700/60 transition-colors" title="Executar este passo">
  <i class="pi pi-play text-[10px]"></i>
</button>
```

* **Regra de Títulos e Ações com Texto:** Quando um botão contiver texto (ex: botões primários de formulário ou criação), deve conter no máximo **1 a 2 palavras** (ex: *Novo Agente*, *Salvar*, *Executar*, *Enviar*). Omitir redundâncias de texto quando o ícone já transmitir a ação claramente.

## 5. Contadores e Métricas em Cards (Alinhamento à Direita com Tooltip)

* Métricas secundárias (execuções, falhas, etc.) em cards de listagem devem ser agrupadas à direita, imediatamente à esquerda da data da última execução.

* Omitir rótulos extensos ("Execuções", "Erros") e exibir somente o ícone PrimeIcon + número, utilizando a propriedade `title` como tooltip.

```html
<div class="flex items-center gap-3 shrink-0 text-[10px]">
  <div class="flex items-center gap-2.5 bg-dark-900/60 px-2 py-0.5 rounded border border-stone-800/80">
    <span class="inline-flex items-center gap-1 text-stone-300" title="Execuções">
      <i class="pi pi-bolt text-amber-500 text-[10px]"></i>
      <span class="font-medium font-mono text-[11px]">{{ agent.total_runs || 0 }}</span>
    </span>
    <span class="inline-flex items-center gap-1 text-stone-300" title="Erros">
      <i class="pi pi-exclamation-triangle text-rose-400 text-[10px]"></i>
      <span class="font-medium font-mono text-[11px]">{{ agent.failed_runs || 0 }}</span>
    </span>
  </div>

  <div class="flex items-center gap-1 text-stone-500" title="Última Execução">
    <i class="pi pi-clock text-[10px]"></i>
    <span>{{ formatDate(agent.updated_at || agent.created_at) }}</span>
  </div>
</div>
```

## 6. Menu de Ações por Três Pontinhos & Badges de Status (Padrão N8N)

* Ações secundárias em itens de listas de agentes e fluxos são consolidadas em um menu dropdown acionado pelo ícone de três pontinhos verticais (`pi pi-ellipsis-v`).

* Ao lado do menu de três pontinhos, utiliza-se a badge de status (Publicado = verde `emerald-400`, Não Publicado = cinza `stone-400`, Arquivado = âmbar `amber-400`).

* Ação de arquivar remove o item dos ativos sem exigir dialog de confirmação.

```html
<!-- Badge de Status -->
<span class="px-2 py-0.5 rounded-full text-[10px] font-medium bg-emerald-950/80 text-emerald-400 border border-emerald-800/80 inline-flex items-center gap-1.5">
  <span class="w-1.5 h-1.5 rounded-full bg-emerald-400 animate-pulse"></span>
  <span>Publicado</span>
</span>

<!-- Botão Três Pontinhos -->
<button class="p-1.5 text-stone-400 hover:text-stone-100 bg-stone-800 hover:bg-stone-700 border border-stone-700/60 rounded text-xs w-7 h-7 inline-flex items-center justify-center">
  <i class="pi pi-ellipsis-v text-xs"></i>
</button>
```

## 7. Barra de Controles e Filtros Minimalista

* Listas de cartões utilizam barras de controle superiores limpas e sem caixas/bordas envolventes (`flex flex-wrap items-center justify-between gap-3 shrink-0 px-1 py-1`).

* Integração direta de: Título contextual à esquerda + Busca rápida (`w-60`), Ordenação e Botão Funil de Status à direita.

## 8. Regra de Palavras por Ação

* **No Construtor / Editor Visual:** Estritamente **1 palavra** por botão (*Subagente*, *Limpar*, *Código*, *Coach*, *Playground*, *Executar*, *Publicar*, *Lista*, *Agente*, *Execuções*, *Reverter*).

* **Na Navegação Geral e Modais:** Máximo de **2 palavras** (*Novo Agente*, *Ver Código*, *Ver Logs*).

## 9. Botão Dropdown Split e Painéis Deslizantes (Drawers)

* Ações primárias com sub-opções utilizam botões split compactos com chevron (`pi pi-chevron-down`).

* Menus dropdown suspensos utilizam `bg-dark-900 border border-stone-700/80 rounded-md shadow-2xl z-40 py-1 text-xs text-stone-200 divide-y divide-stone-800/80`.

* Painéis de inspeção e histórico utilizam gavetas deslizantes laterais (`transition ease-out duration-200`, fixadas ou absolute na direita) com cabeçalho escuro e botões de ação de ícone único com tooltip.

## 10. Exemplo de Componente Vue 3 (Composition API)

```vue
<template>
  <button class="bg-gradient-to-r from-orange-700 to-amber-600 hover:from-orange-600 hover:to-amber-500 text-white rounded-md text-xs font-semibold px-3 py-1.5 transition-all shadow-md shadow-orange-950/40 flex items-center space-x-1.5 cursor-pointer">
    <i class="pi pi-play"></i>
    <span>Executar</span>
  </button>
</template>
```
