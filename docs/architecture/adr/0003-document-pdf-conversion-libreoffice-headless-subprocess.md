# ADR 0003: Estratégia de Execução para Conversão de Documentos em PDF (.doc/.docx → .pdf)

* **Status:** Aceito
* **Data:** 2026-09-09
* **Autor:** Arquiteto de Software

---

## 1. Contexto
A família 3 de conversão exige a transformação de documentos de texto legados e modernos (`.doc` e `.docx`) para o formato `.pdf`. A fidelidade visual (fontes, margens, tabelas, cabeçalhos e elementos gráficos) é um critério essencial de aceite do produto.

---

## 2. Opções Consideradas

1. **Opção A: Parser e Gerador em Go Puro / Unioffice**
   - *Prós:* Sem dependência de softwares desktop no container.
   - *Contras:* Bibliotecas abertas em Go não possuem motor de renderização de layout completo para calcular a paginação e quebra de linhas exata de documentos Microsoft Word (`.doc`/`.docx`), gerando inconsistências visuais inaceitáveis.

2. **Opção B: Integrador via API / Serviços Proprietários de Terceiros**
   - *Prós:* Não exige poder computacional do servidor da API.
   - *Contras:* Viola a premissa de projeto Open Source autocontido sem dependências de serviços externos pagos ou proprietários.

3. **Opção C: LibreOffice Headless via Subprocesso no Container**
   - *Prós:*
     - Motor de renderização Open Source mais maduro e preciso para documentos office.
     - Suporte nativo ao comando CLI `libreoffice --headless --convert-to pdf input.docx`.
     - Execução isolada em container via `os/exec.CommandContext`.
   - *Contras:*
     - Imagem Docker maior devido ao footprint do LibreOffice (~250-400MB).
     - Inicialização do processo LibreOffice consome mais memória e tempo (~1 a 3 segundos por documento).

---

## 3. Decisão
Adotar a **Opção C (LibreOffice Headless via Subprocesso no Container)**.

O adaptador `LibreOfficeDocConverter` executará o comando LibreOffice em modo headless dentro de um diretório temporário isolado por job (`/tmp/job-{id}`). O container do worker conterá as fontes básicas padrão (ex: `ttf-mscorefonts-installer` / Dejavu Fonts) para garantir renderização consistente.

---

## 4. Consequências

* **Positivas:**
  - Garantia de alta fidelidade visual na conversão de documentos `.doc` e `.docx` para `.pdf`.
  - Atendimento rigoroso aos critérios de aceite funcionais definidos em `requirements.md`.
  - Manutenção da premissa Open Source e desacoplamento de fornecedores externos.

* **Negativas / Mitigações:**
  - *Tamanho do Container:* O worker exigirá uma imagem Docker com LibreOffice headless instalado. (Mitigação: separar a imagem Docker da API REST leve da imagem Docker do Worker pesado de conversão).
  - *Tempo de Execução:* Documentos grandes podem demorar mais para processar. (Mitigação: o SLA de 60s estabelecido no RNF acomoda confortavelmente o tempo de execução do LibreOffice em modo headless).
