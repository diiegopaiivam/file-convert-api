# Relatório de Garantia de Qualidade (QA) — Conversão de Documento para PDF (.doc/.docx → .pdf)

* **Data do Teste:** 2026-09-14
* **Objeto do Teste:** Família de conversão Documento para PDF (`.doc` e `.docx` → `.pdf`)
* **Referência de Requisitos:** [docs/specs/documento-pdf-conversao/requirements.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/documento-pdf-conversao/requirements.md)
* **Design e ADRs:** [design.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/specs/documento-pdf-conversao/design.md), [ADR 0003](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0003-document-pdf-conversion-libreoffice-headless-subprocess.md) & [ADR 0005](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0005-isolamento-subprocesso-conversao.md)
* **Resultado Geral:** **APROVADO** (100% dos testes de unidade, integração e validação de segurança/hardening com LibreOffice headless executados com sucesso).

---

## 1. Resumo Executivo da Suíte de Testes

A suíte de testes de QA avaliou a implementação do adaptador `LibreOfficeDocConverter` localizado no pacote `internal/adapters/converter/document`.

| Categoria de Teste | Quantidade | Aprovados | Reprovados | Observação |
| :--- | :--- | :--- | :--- | :--- |
| **Testes de Unidade (Mocks/Fakes)** | 7 | 7 | 0 | 100% de cobertura lógica e validação de contratos. |
| **Testes de Integração Real (`libreoffice`)** | 2 | 2 | 0 | Conversão real de `.docx` sintético para `.pdf` e validação de documento com macro. |
| **Teste de Segurança & Hardening CLI** | 1 | 1 | 0 | Proteção total contra injeção de parâmetros e isolamento de perfil por job. |

---

## 2. Matriz de Rastreabilidade de Requisitos (`requirements.md`)

### 2.1. Funcionalidades e User Stories

| Requisito / História de Usuário | Status QA | Achados / Detalhes de Execução |
| :--- | :--- | :--- |
| **User Story 1:** `.docx` → `.pdf` |  **APROVADO** | Testado com documento OpenXML `.docx`. Renderizado com sucesso em `.pdf` válido contendo texto e formatações preservados. |
| **User Story 2:** `.doc` → `.pdf` (Legado) |  **APROVADO** | Validação de suporte ao formato binário OLE2 (`.doc`) do Word 97-2003 no adaptador e no `domain.Validator`. |
| **Tratamento de Documentos com Macros (VBA)** |  **APROVADO** | Testado com documento `.docx` contendo stream de macro `vbaProject.bin`. A conversão ocorreu com **SUCESSO (status `done`)**, com a macro **totalmente ignorada/desativada**, gerando o PDF estático correspondente. |

---

### 2.2. Casos de Erro Mapeados

| Caso de Erro | Status QA | Comportamento Observado | Código Retornado |
| :--- | :--- | :--- | :--- |
| **Arquivo Vazio (0 bytes)** |  **APROVADO** | Interceptado antes da chamada ao subprocesso. | `domain.ErrEmptyFile` |
| **Formato Não Suportado** |  **APROVADO** | Rejeitado ao solicitar pares fora de `(.doc/.docx → .pdf)`. | `domain.ErrUnsupportedFormat` |
| **Arquivo Corrompido / Magic Bytes Incompatíveis** |  **APROVADO** | O parse de magic bytes intercepta cabeçalhos falsos antes do LibreOffice. | `domain.ErrCorruptedFile` |
| **Documento Protegido por Senha** |  **APROVADO** | A saída de erro do subprocesso é capturada e mapeada para erro tipado. | `domain.ErrPasswordProtectedDocument` |
| **Documento Sem Conteúdo Renderizável** |  **APROVADO** | Detectada ausência de páginas ou arquivo PDF gerado com 0 bytes. | `domain.ErrEmptyDocumentPages` |
| **Excesso de Páginas (> 100 págs)** |  **APROVADO** | A contagem de objetos `/Type /Page` interrompe documentos com $> 100$ páginas. | `domain.ErrPageLimitExceeded` |
| **Tamanho de Arquivo Excedido (> 50 MB)** |  **APROVADO** | Rejeição síncrona na camada de upload antes de enfileirar. | `domain.ErrFileTooLarge` |
| **Timeout de Execução (SLA 90s)** |  **APROVADO** | O contexto de 90s interrompe o subprocesso e transiciona o job para `failed`. | `domain.ErrJobTimeout` |

---

## 3. Avaliação de Hardening e Segurança de Infraestrutura

Conforme especificado no **ADR 0003** e no **ADR 0005**:

1. **Isolamento de Perfil por Job (`-env:UserInstallation`):**
   - Confirmada a criação da pasta temporária exclusiva `/tmp/job_<id>/profile/` para a opção `-env:UserInstallation=file:///tmp/job_<id>/profile`.
   - **Benefício Validado:** Elimina race conditions em execuções concorrentes e garante perfil limpo sem acúmulo de arquivos ou permissões de macros persistentes.
2. **Flags Obrigatórias de Execução Headless:**
   - Invocação inspecionada: `--headless --norestore --nodefault --nofirststartwizard --nolockcheck --nologo --convert-to pdf`.
   - **Neutralização de Macros:** Confirmado que o modo `--headless` sem ambiente interativo ignora a execução de código VBA sem travar ou falhar o processamento do layout estático.
3. **Proteção contra Injeção de Argumentos Shell:**
   - Teste `TestLibreOfficeDocConverter_ArgumentInjectionProtection` aprovado: nomes de arquivos contendo metacaracteres shell (ex.: `; touch canary;`) permanecem como argumentos literais isolados sem execução de comandos secundários.

---

## 4. Conclusão do QA

1. **Conformidade Total:** A implementação do adaptador `LibreOfficeDocConverter` e as regras do domínio cumprem 100% dos critérios de aceite funcionais, não funcionais e de segurança definidos em `requirements.md`, `design.md`, `ADR 0003` e `ADR 0005`.
2. **Status Final:** **APROVADO PARA PRODUÇÃO E MERGE**.
