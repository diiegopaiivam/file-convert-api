# ADR 0002: Estratégia de Execução para Conversão de Imagem e PDF (.jpg/.png ↔ .pdf)

* **Status:** Aceito
* **Data:** 2026-09-09
* **Autor:** Arquiteto de Software

---

## 1. Contexto
A família 2 de conversão exige suporte a:
1. Imagem para PDF (`.jpg`, `.png` → `.pdf`)
2. PDF para Imagem (`.pdf` → `.jpg`)

Essa conversão requer manipulação visual, rasterização de páginas vetoriais PDF para mapas de bits JPG, e encapsulamento de imagens em documentos PDF com preservação de dimensões e densidade de pixels (DPI).

---

## 2. Opções Consideradas

1. **Opção A: Bibliotecas Nativas em Go (Pure Go ex: `gofpdf`, `pdfcpu`)**
   - *Prós:* Sem dependências de binários externos do sistema operacional.
   - *Contras:* Bibliotecas de PDF em Go possuem limitações severas para rasterizar e renderizar páginas PDF complexas em imagens JPG sem bibliotecas nativas de renderização (como Cairo/Poppler).

2. **Opção B: Ferramentas CLI Utilitárias no Container (`ImageMagick` + `Poppler-utils`)**
   - *Prós:* 
     - Utilização do `pdftoppm` (parte do pacote Poppler) para renderizar páginas PDF em JPG com extrema qualidade e fidelidade visual.
     - Utilização do ImageMagick (`magick` / `convert`) para transformar imagens JPG/PNG em arquivos PDF padronizados.
     - Isolamento completo de memória via subprocesso `os/exec.CommandContext`.
   - *Contras:* Necessidade de empacotar `imagemagick` e `poppler-utils` no container da aplicação.

---

## 3. Decisão
Adotar a **Opção B (Ferramentas CLI Utilitárias `ImageMagick` e `Poppler-utils` via Subprocesso)**.

- Para `.jpg`/`.png` → `.pdf`: O adaptador utilizará o comando `magick convert input.jpg output.pdf` com políticas de segurança restritas para execução em servidor (*policy.xml* ajustado).
- Para `.pdf` → `.jpg`: O adaptador utilizará `pdftoppm -jpeg -r 150 input.pdf output_page`, extraindo as páginas para imagens JPG e empacotando em arquivo `.zip` caso o resultado contenha múltiplas páginas.

---

## 4. Consequências

* **Positivas:**
  - Qualidade de renderização gráfica e conversão com padrão de mercado.
  - Controle preciso de timeout e cancelamento de processos pesados de conversão visual via contexto Go.
  - Segurança contra explorações de arquivos maliciosos através da isolação no container e configuração das políticas do ImageMagick.

* **Negativas / Mitigações:**
  - *Consumo de Memória:* Processamento de imagens grandes pode exigir limites razoáveis de RAM no container (Mitigação: limitar o tamanho de upload a 50 MB e definir limites de memória nos containers Docker/K8s).
