# Design de Arquitetura — Feature: Conversão de Áudio (.wav ↔ .mp3)

## 1. Visão Geral da Feature
A feature **Audio Conversion** é a primeira entrega funcional da FileConvert API (v1). Ela introduz a infraestrutura base da API assíncrona e entrega a conversão bidirecional entre arquivos de áudio nos formatos `.wav` e `.mp3`.

---

## 2. Componentes e Fluxo de Execução

### 2.1. Componentes Envolvidos
- **HTTP Handler (`AudioConversionHandler`)**: Recebe a requisição multipart/form-data com o arquivo de áudio e os parâmetros de conversão.
- **ObjectStorage Adapter**: Armazena o arquivo de origem original (`source`) e o arquivo convertido gerado (`target`).
- **JobRepository Adapter**: Persiste os metadados do job e atualiza o ciclo de vida dos estados (`pending` → `processing` → `done` / `failed`).
- **JobQueue Adapter**: Notifica os workers assíncronos sobre novos jobs pendentes.
- **Audio Worker / `FFmpegAudioConverter`**: Consome o job da fila, baixa o áudio do storage, executa o `ffmpeg` via subprocesso CLI isolado no container, faz upload do resultado e finaliza o job.

### 2.2. Diagrama de Sequência

```
Client              API HTTP Handler         ObjectStorage         JobRepository        JobQueue           Audio Worker / FFmpeg
  |                        |                       |                     |                  |                        |
  |--- POST /conversions ->|                       |                     |                  |                        |
  |    (audio file)        |-- Upload Source ----->|                     |                  |                        |
  |                        |<-- return key --------|                     |                  |                        |
  |                        |-- Create Job (pending) -------------------->|                  |                        |
  |                        |-- Enqueue JobID ---------------------------------------------->|                        |
  |<-- 202 (job_id) -------|                       |                     |                  |                        |
  |                        |                       |                     |                  |<-- Dequeue JobID ------|
  |                        |                       |                     |-- Status: proc ->|                        |
  |                        |                       |<-- Download Source -|                  |                        |
  |                        |                       |                     |                  |-- Run FFmpeg CLI ----->|
  |                        |                       |                     |                  |<-- Return Output Stream|
  |                        |                       |-- Upload Target --->|                  |                        |
  |                        |                       |                     |-- Status: done ->|                        |
  |                        |                       |                     |                  |-- Ack Queue Message -->|
  |                        |                       |                     |                  |                        |
  |--- GET /jobs/{id} ---->|                       |                     |                  |                        |
  |<-- 200 (status: done)--|                       |                     |                  |                        |
```

---

## 3. Especificação dos Endpoints HTTP

### 3.1. Submeter Conversão de Áudio
- **Método**: `POST`
- **Path**: `/v1/conversions/audio`
- **Content-Type**: `multipart/form-data`
- **Parâmetros do Form**:
  - `file`: Arquivo binário de áudio (`.wav` ou `.mp3`).
  - `target_format`: String (`mp3` ou `wav`).

**Resposta de Sucesso (`202 Accepted`):**
```json
{
  "job_id": "job_123456789",
  "source_format": "wav",
  "target_format": "mp3",
  "status": "pending",
  "created_at": "2026-09-09T18:00:00Z"
}
```

### 3.2. Consultar Status do Job
- **Método**: `GET`
- **Path**: `/v1/jobs/{job_id}`

**Resposta de Job Concluído (`200 OK`):**
```json
{
  "job_id": "job_123456789",
  "source_format": "wav",
  "target_format": "mp3",
  "status": "done",
  "download_url": "/v1/jobs/job_123456789/download",
  "created_at": "2026-09-09T18:00:00Z",
  "updated_at": "2026-09-09T18:00:05Z"
}
```

**Resposta de Job Com Falha (`200 OK`):**
```json
{
  "job_id": "job_123456789",
  "source_format": "wav",
  "target_format": "mp3",
  "status": "failed",
  "error": {
    "code": "CORRUPTED_FILE",
    "message": "O arquivo de áudio enviado está corrompido e não pôde ser decodificado pelo FFmpeg.",
    "details": [
      {
        "issue": "FFmpeg exit status 1: Invalid data found when processing input"
      }
    ]
  },
  "created_at": "2026-09-09T18:00:00Z",
  "updated_at": "2026-09-09T18:00:02Z"
}
```

### 3.3. Download do Arquivo Convertido
- **Método**: `GET`
- **Path**: `/v1/jobs/{job_id}/download`
- **Resposta (`200 OK`)**: Stream binário de áudio com cabeçalho `Content-Type: audio/mpeg` (para mp3) ou `audio/wav` (para wav).

---

## 4. Implementação do Adaptador `FFmpegAudioConverter`

O adaptador de áudio implementará o port `port.Converter`:

```go
type FFmpegAudioConverter struct {
	ffmpegPath string
}

func NewFFmpegAudioConverter(ffmpegPath string) *FFmpegAudioConverter {
	return &FFmpegAudioConverter{ffmpegPath: ffmpegPath}
}

func (c *FFmpegAudioConverter) CanConvert(src, target domain.Format) bool {
	validWavToMp3 := (src == domain.FormatWAV && target == domain.FormatMP3)
	validMp3ToWav := (src == domain.FormatMP3 && target == domain.FormatWAV)
	return validWavToMp3 || validMp3ToWav
}
```

### Comando Subprocesso FFmpeg:
- Para WAV → MP3: `ffmpeg -i input.wav -codec:a libmp3lame -qscale:a 2 output.mp3`
- Para MP3 → WAV: `ffmpeg -i input.mp3 -codec:a pcm_s16le output.wav`

O contexto `ctx` deve ter um timeout configurado de no máximo 60 segundos para honrar o SLA do RNF.

---

## 5. Estratégia de Testes e Validação
1. **Testes Unitários**:
   - Validação da lógica do `CanConvert`.
   - Teste do `FFmpegAudioConverter` utilizando pequenos arquivos senoidais sintéticos gerados em memória ou fixtures de 1 segundo.
2. **Testes de Integração**:
   - Submissão de chamadas HTTP reais enviando arquivos `.wav` e `.mp3` válidos, acompanhando a transição do job de `pending` para `done`.
   - Validação de casos de erro: envio de arquivo corrompido (string aleatória salva como `.wav`), arquivo de 0 bytes, e arquivo com extensão inválida.
