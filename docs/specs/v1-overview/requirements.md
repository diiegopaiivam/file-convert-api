# Requisitos e Especificações Funcionais — FileConvert API (v1)

## 1. Visão Geral e Escopo
A **FileConvert API** é uma API REST assíncrona para conversão de arquivos entre formatos populares sem exigir a instalação de bibliotecas locais por parte dos clientes que a consomem.

O escopo da versão v1 contempla três famílias de conversão:
- **Áudio**: Bi-direcional `.wav` ↔ `.mp3`
- **Imagem ↔ PDF**: `.jpg` / `.png` → `.pdf` e `.pdf` → `.jpg`
- **Documento → PDF**: `.doc` / `.docx` → `.pdf`

Toda conversão ocorre exclusivamente de forma **assíncrona**: o cliente submete o arquivo original, recebe um identificador único de job (`job_id`), e posteriormente consulta o status do job (ou recebe uma notificação via webhook) para obter o arquivo convertido. Não existe suporte a conversão síncrona na versão v1.

---

## 2. Histórias de Usuário e Critérios de Aceite

### 2.1. Família 1: Conversão de Áudio (`.wav` ↔ `.mp3`)
**História de Usuário:**
- **Como** consumidor da API,
- **Quero** enviar um arquivo de áudio no formato `.wav` ou `.mp3` solicitando a conversão para o formato oposto (`.mp3` ou `.wav`),
- **Para que** eu possa otimizar o armazenamento ou garantir compatibilidade com meus reprodutores de mídia.

**Cenário 1: Conversão de `.wav` para `.mp3` com sucesso**
- **Given** que o cliente possui um arquivo de áudio `.wav` válido e de tamanho dentro do limite permitido,
- **When** o cliente envia uma requisição de conversão definindo o formato de destino como `mp3`,
- **Then** a API aceita a requisição, retorna resposta de confirmação com um `job_id` e status inicial `pending`,
- **And** quando o processamento for concluído, o status do job deve transicionar para `done` disponibilizando o recurso para download do arquivo `.mp3` gerado.

**Cenário 2: Conversão de `.mp3` para `.wav` com sucesso**
- **Given** que o cliente possui um arquivo de áudio `.mp3` válido,
- **When** o cliente envia a solicitação de conversão para `wav`,
- **Then** a API registra o job com status `pending`,
- **And** ao finalizar o processamento, o status muda para `done`, disponibilizando um arquivo `.wav` válido.

**Critério de Aceite ("Quando está pronto"):**
- O job atinge o estado final `done`.
- O arquivo retornado é um binário de áudio válido no formato solicitado (`.mp3` ou `.wav`), mantendo a integridade sonora do arquivo original e reproduzível sem falhas de decodificação.

---

### 2.2. Família 2: Conversão de Imagem e PDF (`.jpg`/`.png` ↔ `.pdf`)
**História de Usuário:**
- **Como** consumidor da API,
- **Quero** converter imagens (`.jpg` ou `.png`) em documentos `.pdf`, ou extrair conteúdo de um `.pdf` para imagens `.jpg`,
- **Para que** eu possa consolidar imagens em documentos legíveis ou converter páginas de PDF para processamento visual.

**Cenário 1: Conversão de imagem (`.jpg` / `.png`) para `.pdf`**
- **Given** que o cliente envia uma imagem válida no formato `.jpg` ou `.png`,
- **When** o cliente solicita a conversão para o formato de destino `pdf`,
- **Then** a API responde confirmando o recebimento com o `job_id` e status `pending`,
- **And** após o processamento, o job muda para `done`, gerando um documento `.pdf` contendo a imagem renderizada corretamente.

**Cenário 2: Conversão de `.pdf` para imagem (`.jpg`)**
- **Given** que o cliente envia um arquivo `.pdf` válido,
- **When** o cliente solicita a conversão para `jpg`,
- **Then** a API processa o job de forma assíncrona,
- **And** ao finalizar, o job muda para `done`, disponibilizando a imagem resultante (ou conjunto de imagens por página) no formato `.jpg`.

**Critério de Aceite ("Quando está pronto"):**
- O job atinge o estado final `done`.
- Na conversão para PDF: o arquivo baixado abre em leitores padrão de PDF exibindo a imagem original com proporções e orientação preservadas.
- Na conversão para JPG: a imagem resultante é gerada com resolução legível correspondente às páginas do PDF original.

---

### 2.3. Família 3: Conversão de Documento para PDF (`.doc`/`.docx` → `.pdf`)
**História de Usuário:**
- **Como** consumidor da API,
- **Quero** submeter um documento de texto (`.doc` ou `.docx`),
- **Para que** ele seja convertido para `.pdf` garantindo a imutabilidade do layout e facilidade de distribuição.

**Cenário 1: Conversão de `.docx` / `.doc` para `.pdf` com sucesso**
- **Given** que o cliente possui um documento `.docx` ou `.doc` válido contendo texto e formatações,
- **When** o cliente faz o upload do documento solicitando a conversão para `pdf`,
- **Then** a API retorna um `job_id` com status `pending`,
- **And** ao finalizar o processamento, o status transiciona para `done`, fornecendo o acesso ao documento `.pdf` final.

**Critério de Aceite ("Quando está pronto"):**
- O job atinge o estado final `done`.
- O documento `.pdf` gerado preserva o texto, estrutura de páginas, tabelas e elementos visuais do documento original sem quebra de formatação.

---

## 3. Casos de Erro Obrigatórios por Família

Todas as três famílias de conversão devem tratar e cobrir os quatro cenários de erro abaixo:

| Caso de Erro | Estágio da Detecção | Comportamento Esperado | Status / Resposta |
| :--- | :--- | :--- | :--- |
| **Arquivo Vazio (0 bytes)** | Upload (Recepção) | Rejeitar a requisição imediatamente no momento do recebimento, sem criar job de conversão. | Resposta de erro imediata com código `EMPTY_FILE`. |
| **Tamanho Acima do Limite** | Upload (Recepção) | Interromper a recepção do arquivo assim que o limite configurado for atingido ou declarado no cabeçalho. | Resposta de erro imediata com código `FILE_TOO_LARGE`. |
| **Formato de Origem Não Suportado** | Validação de Entrada | Verificar o cabeçalho/extensão do arquivo e rejeitar a solicitação caso não pertença ao escopo da versão v1. | Resposta de erro imediata com código `UNSUPPORTED_FORMAT`. |
| **Arquivo Corrompido** | Processamento Assíncrono | Aceitar o upload (`pending`), porém registrar a falha durante a execução do parsing/conversão. | Transição do job para o estado `failed` com código `CORRUPTED_FILE`. |

---

## 4. Requisitos Não Funcionais (RNF)

### 4.1. Limite Máximo de Tamanho de Arquivo
- **Limite por Upload:** O tamanho máximo permitido para qualquer arquivo enviado é de **50 MB**.
- **Validação:** Requisições com arquivos que excedam 50 MB devem ser rejeitadas na entrada antes do agendamento do processamento.

### 4.2. Tempo Máximo de Transição do Job (SLA)
- **Tempo Limite (Timeout):** O tempo decorrido desde o momento em que o job entra no estado `pending` até a sua transição para o estado final (`done` ou `failed`) não deve exceder **60 segundos**.
- **Tratamento de Excesso de Tempo:** Caso o processamento não seja concluído em até 60 segundos, o sistema deve automaticamente alterar o status do job para `failed` com a causa `JOB_TIMEOUT`.

### 4.3. Formato Padrão de Resposta de Erro da API
Todas as respostas de erro geradas pela API (seja na recepção síncrona da requisição ou no payload de consulta a um job no status `failed`) devem seguir a seguinte estrutura padronizada em JSON:

```json
{
  "code": "CORRUPTED_FILE",
  "message": "O arquivo enviado está corrompido e não pôde ser processado.",
  "details": [
    {
      "field": "file",
      "issue": "Estrutura de cabeçalho do arquivo inválida para o formato declarado."
    }
  ],
  "timestamp": "2026-09-09T18:00:00Z"
}
```

**Especificação dos Campos:**
- `code` *(string, obrigatório)*: Código textual único e estável que identifica a categoria do erro (ex: `EMPTY_FILE`, `FILE_TOO_LARGE`, `UNSUPPORTED_FORMAT`, `CORRUPTED_FILE`, `JOB_TIMEOUT`).
- `message` *(string, obrigatório)*: Descrição amigável e explicativa do erro em linguagem natural.
- `details` *(array, opcional)*: Informações complementares com detalhes contextuais da falha.
- `timestamp` *(string, obrigatório)*: Carimbo de data/hora da ocorrência no formato ISO-8601 em UTC.

---

## 5. Recomendação de Prioridade de Entrega

Recomenda-se a implementação das três famílias na seguinte ordem de prioridade:

1. **1ª Prioridade: Áudio (`.wav` ↔ `.mp3`)**
   - **Justificativa:** A conversão de áudio possui regras de validação diretas e fluxo binário bem delimitado. Serve como o cenário ideal para construir e validar toda a infraestrutura base do ciclo de vida assíncrono (recepção do arquivo, criação do `job_id`, transições de status `pending` → `done`/`failed` e recuperação do resultado) com menor sobrecarga de renderização visual.

2. **2ª Prioridade: Imagem e PDF (`.jpg`/`.png` ↔ `.pdf`)**
   - **Justificativa:** Com o fluxo assíncrono da API estabilizado na 1ª fase, a conversão de imagem/PDF introduz o manuseio de arquivos visuais de múltiplas páginas e métricas de dimensão/proporção sem a complexidade adicional de interpretação de layouts complexos de texto e fontes.

3. **3ª Prioridade: Documentos para PDF (`.doc`/`.docx` → `.pdf`)**
   - **Justificativa:** É a família com maior grau de complexidade em termos de preservação de fidelidade visual, paginação, tipografia e elementos de formatação. Deixar esta família por último permite focar na qualidade da conversão após os mecanismos de resiliência, limites e respostas de erro da API já estarem consolidados e testados nas famílias anteriores.
