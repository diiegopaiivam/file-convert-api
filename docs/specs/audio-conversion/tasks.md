# Lista de Tarefas (Tasks) — Feature: Conversão de Áudio (.wav ↔ .mp3)

> **Destinado a:** Desenvolvedor Senior Go  
> **Objetivo:** Implementação completa da feature de conversão de áudio assíncrona, cobrindo adaptadores, suporte a FFmpeg no container, manipuladores HTTP e testes de integração.

---

## Task List Breakdown

### Task 1: Estruturas de Domínio e Ports Hexagonais
- [ ] Criar o pacote `domain` com as entidades `Job`, `JobStatus`, `Format` e `JobError`.
- [ ] Implementar a estrutura `APIErrorResponse` e os construtores de erro padronizados (`EMPTY_FILE`, `FILE_TOO_LARGE`, `UNSUPPORTED_FORMAT`, `CORRUPTED_FILE`, `JOB_TIMEOUT`).
- [ ] Criar o pacote `port` definindo as interfaces em Go para `Converter`, `ConverterRegistry`, `JobRepository`, `ObjectStorage` e `JobQueue` conforme especificado em [ports.md](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/ports.md).
- [ ] **Critério de Aceite da Task:** Código compila sem erros, tipos expostos corretamente sem dependências externas de infraestrutura no domínio.

---

### Task 2: Implementar o Adaptador `FFmpegAudioConverter`
- [ ] Criar o adaptador `FFmpegAudioConverter` que implementa a interface `port.Converter`.
- [ ] Implementar o método `CanConvert(src, target domain.Format)` para validar pares `(.wav ↔ .mp3)`.
- [ ] Implementar o método `Convert(ctx context.Context, src io.Reader, srcFormat, targetFormat domain.Format)` executando o binário `ffmpeg` via `os/exec.CommandContext`.
- [ ] Garantir o encerramento do subprocesso filho caso o `ctx` seja cancelado ou atinja o timeout de 60 segundos.
- [ ] Tratar falhas de execução do `ffmpeg` capturando o `stderr` e retornando erro categorizado como `CORRUPTED_FILE` em falhas de decodificação.
- [ ] Criar testes unitários em `adapter/converter/ffmpeg_audio_test.go` utilizando fixtures de áudio sintético.
- [ ] **Critério de Aceite da Task:** Testes unitários do conversor passando com cobertura de sucesso e falha (arquivo de áudio inválido/corrompido).

---

### Task 3: Implementar o `ConverterRegistry`
- [ ] Criar o componente `ConverterRegistry` responsável por armazenar e buscar a implementação de `Converter` adequada em runtime.
- [ ] Implementar os métodos `Register(converter Converter)` e `GetConverter(srcFormat, targetFormat domain.Format)`.
- [ ] Escrever testes unitários em `registry/converter_registry_test.go` validando busca com sucesso e retorno de `UNSUPPORTED_FORMAT` para formatos desconhecidos.
- [ ] **Critério de Aceite da Task:** Cobertura de testes unitários validando resolução de adaptadores em runtime.

---

### Task 4: Implementar Manipuladores HTTP e Validações Síncronas
- [ ] Criar o endpoint `POST /v1/conversions/audio` aceitando payloads `multipart/form-data`.
- [ ] Adicionar validação síncrona imediata para:
  - Arquivo com 0 bytes → Retornar HTTP 400 com `code: "EMPTY_FILE"`.
  - Tamanho excede 50 MB → Retornar HTTP 413 com `code: "FILE_TOO_LARGE"`.
  - Formato não suportado (diferente de `wav` e `mp3`) → Retornar HTTP 415 com `code: "UNSUPPORTED_FORMAT"`.
- [ ] Criar endpoints de consulta de status: `GET /v1/jobs/{job_id}` e download `GET /v1/jobs/{job_id}/download`.
- [ ] **Critério de Aceite da Task:** Respostas HTTP síncronas atendem rigorosamente à estrutura de JSON de erro padronizada definida no RNF.

---

### Task 5: Implementar o Worker Assíncrono de Áudio
- [ ] Criar o loop consumidor do worker em `worker/audio_worker.go`.
- [ ] O worker deve:
  1. Retirar um `job_id` da `JobQueue`.
  2. Atualizar o status do job para `processing`.
  3. Baixar o arquivo de origem via `ObjectStorage`.
  4. Executar a conversão via `ConverterRegistry` / `FFmpegAudioConverter` com timeout limite de 60s (`JOB_TIMEOUT`).
  5. Enviar o arquivo resultante para o `ObjectStorage`.
  6. Atualizar o status do job para `done` (com o `target_object_key`) ou `failed` (com a estrutura `JobError`).
  7. Confirmar a mensagem na fila (`ack()`).
- [ ] **Critério de Aceite da Task:** Processamento assíncrono executa o ciclo completo de vida do job sem travamentos ou vazamentos de goroutines.

---

### Task 6: Atualizar Dockerfile e Containerização
- [ ] Adicionar o pacote `ffmpeg` ao `Dockerfile` da aplicação.
- [ ] Configurar variáveis de ambiente para caminhos dos binários nativos.
- [ ] **Critério de Aceite da Task:** Imagem Docker compila com sucesso e o binário `ffmpeg` está acessível no PATH do container.

---

### Task 7: Testes de Integração End-to-End (E2E)
- [ ] Escrever suite de testes de integração em `tests/integration/audio_conversion_test.go`.
- [ ] Testar fluxo feliz: Upload `.wav` → Polling do job até `status: done` → Download e validação do `.mp3`.
- [ ] Testar fluxo de erro assíncrono: Upload de arquivo corrompido com extensão `.wav` → Polling do job até `status: failed` com código `CORRUPTED_FILE`.
- [ ] **Critério de Aceite da Task:** Todos os testes de integração passando via `go test -tags=integration ./...`.
