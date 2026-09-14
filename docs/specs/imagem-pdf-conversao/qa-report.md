# Relatório de Garantia de Qualidade (QA) — Conversão Imagem ↔ PDF

* **Data do Teste:** 2026-09-14
* **Objeto do Teste:** Família de conversão Imagem ↔ PDF (`.jpg`/`.png` → `.pdf` e `.pdf` → `.jpg`)
* **Referência de Requisitos:** [docs/specs/imagem-pdf-conversao/requirements.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/imagem-pdf-conversao/requirements.md)
* **Design e ADRs:** [design.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/imagem-pdf-conversao/design.md) & [ADR 0009](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0009-conversao-imagem-pdf.md)
* **Resultado Geral:** **APROVADO** (Impedimento da política do ImageMagick no container Debian foi corrigido e validado com 100% dos testes de integração passando).

---

## 1. Resumo Executivo da Suíte de Testes

A suíte de testes de QA avaliou os adaptadores `ImageToPDFConverter` e `PDFToImageConverter` implementados no pacote `internal/adapters/converter/image`.

| Categoria de Teste | Quantidade | Aprovados | Reprovados | Observação |
| :--- | :--- | :--- | :--- | :--- |
| **Testes de Unidade (Mocks/Fakes)** | 13 | 13 | 0 | 100% de cobertura lógica e validação de contratos. |
| **Testes de Integração Real (`pdftoppm`)** | 2 | 2 | 0 | Renderização em 300 DPI e empacotamento ZIP aprovados. |
| **Testes de Integração Real (`convert`)** | 3 | 3 | 0 | Validação de geração de PDF a partir de JPG/PNG aprovada após liberação da policy no ImageMagick. |

---

## 2. Matriz de Rastreabilidade de Requisitos (`requirements.md`)

### 2.1. Funcionalidades e User Stories

| Requisito / História de Usuário | Status QA | Achados / Detalhes de Execução |
| :--- | :--- | :--- |
| **User Story 1:** `.jpg`/`.png` → `.pdf` |  **APROVADO** | Conversão de imagens JPG/PNG para PDF via `convert` executada com sucesso após liberação dos direitos no `/etc/ImageMagick-6/policy.xml`. |
| **User Story 2 (Página Única):** `.pdf` → `.jpg` |  **APROVADO** | Testado com `pdftoppm`. PDF de 1 página gera uma imagem `.jpg` individual com densidade de 300 DPI de alta fidelidade visual. |
| **User Story 2 (Multipáginas):** `.pdf` → `.jpg` em `.zip` |  **APROVADO** | PDFs com mais de 1 página têm suas imagens numeradas (`page-1.jpg`, `page-2.jpg`) devidamente compactadas em um arquivo **`.zip`**, validando o modelo de chave única no `TargetObjectKey`. |

---

### 2.2. Casos de Erro Mapeados

| Caso de Erro | Status QA | Comportamento Observado | Código Retornado |
| :--- | :--- | :--- | :--- |
| **Arquivo Vazio (0 bytes)** |  **APROVADO** | Rejeitado antecipadamente na verificação de tamanho de bytes. | `domain.ErrEmptyFile` |
| **Formato Não Suportado** |  **APROVADO** | Rejeitado quando invocada conversão fora do escopo (ex.: `.docx` → `.jpg`). | `domain.ErrUnsupportedFormat` |
| **Imagem / PDF Corrompido** |  **APROVADO** | O parse de magic bytes intercepta headers falsos e falhas de decode. | `domain.ErrCorruptedFile` |
| **PDF Protegido por Senha** |  **APROVADO** | Subprocesso `pdftoppm` detecta encriptação e falha o job com erro tipado. | `domain.ErrPasswordProtectedPDF` |
| **PDF Sem Páginas / Em Branco** |  **APROVADO** | Ausência de imagens renderizadas detectada, evitando exibição de arquivo nulo. | `domain.ErrEmptyPDFPages` |
| **Excesso de Páginas (> 50 págs)** |  **APROVADO** | O adaptador interrompe o empacotamento zip ao atingir 51 páginas. | `domain.ErrPageLimitExceeded` |
| **Timeout de Execução** |  **APROVADO** | Subprocesso interrompido e cancelado ao exceder a janela de contexto. | `domain.ErrJobTimeout` |

---

### 2.3. Requisitos Não Funcionais (RNFs)

- **Densidade de Renderização:** Confirmada a inclusão da flag `-r 300` na chamada do `pdftoppm`, gerando imagens de saída com resolução nítida de 300 DPI.
- **Empacotamento ZIP:** Estrutura e leitura das entradas `.zip` validadas com stdlib `archive/zip`.

---

## 3. Detalhamento da Falha Encontrada (Bug / Impedimento)

### 🐛 Bug #01 — Bloqueio de Política de Segurança no ImageMagick (`policy.xml`) — [CORRIGIDO]

- **Componente:** Adaptador `ImageToPDFConverter` (`internal/adapters/converter/image/image_to_pdf.go`) / Infraestrutura Docker.
- **Descrição Inicial:** Ao tentar converter qualquer imagem `.jpg` ou `.png` em `.pdf` utilizando a instalação padrão do pacote `imagemagick` no Debian Bookworm, o processo `convert` abortava com erro de permissão da `policy.xml`.
- **Causa Raiz:** No Debian Bookworm Slim, o arquivo de configuração `/etc/ImageMagick-6/policy.xml` inclui por padrão a restrição `<policy domain="coder" rights="none" pattern="PDF" />`.
- **Resolução:** O arquivo [deploy/docker/Dockerfile.worker](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/docker/Dockerfile.worker) foi atualizado para ajustar a política de direitos do coder PDF para `rights="read|write"`.
- **Validação:** Todos os testes de integração (`TestIntegration_ImageToPDF_JPGToPDF` e `TestIntegration_ImageToPDF_PNGToPDF`) executados no ambiente containerizado Debian passaram com sucesso (100% de aprovação).

---

## 4. Conclusão e Recomendações do QA

1. **Aprovação Final da Feature:** A implementação dos conversores em Go (`image_to_pdf.go` e `pdf_to_image.go`) e a infraestrutura do `Dockerfile.worker` atendem integralmente a todas as regras de domínio, limites, e contratos de conversão da família Imagem ↔ PDF.
2. **Status:** **PRONTO PARA REVISIONAMENTO E MERGE**.
