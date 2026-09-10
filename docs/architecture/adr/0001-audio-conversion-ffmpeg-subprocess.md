# ADR 0001: Estratégia de Execução para Conversão de Áudio (.wav ↔ .mp3)

* **Status:** Aceito
* **Data:** 2026-09-09
* **Autor:** Arquiteto de Software

---

## 1. Contexto
A família 1 de conversão exige suporte bidirecional entre os formatos de áudio `.wav` e `.mp3`. O processamento de áudio exige codificação/decodificação de codecs (PCM, LAME/MP3), controle de amostragem e integridade do container binário sem comprometer a estabilidade do processo principal da aplicação Go.

---

## 2. Opções Consideradas

1. **Opção A: Bibliotecas Nativas em Go (Pure Go)**
   - *Prós:* Não exige binários externos no container; compilação estática simples.
   - *Contras:* A comunidade Go possui suporte limitado e incompleto para codificação de `.mp3` de alta qualidade em código puro Go sem utilizar bindings C.

2. **Opção B: Bindings Cgo para FFmpeg (`libavcodec` / `libavformat`)**
   - *Prós:* Desempenho em memória elevado sem overhead de criação de processos.
   - *Contras:* Risco elevado de *memory leaks* e *crashes* de memória C que derrubam o processo principal da API Go; compilação cruzada complexa.

3. **Opção C: Execução de Subprocesso CLI do `ffmpeg` no Container**
   - *Prós:* Isolamento total de memória do processo principal; `ffmpeg` é a ferramenta padrão da indústria para manipulação de mídia; fácil controle de timeout via `context.WithTimeout`; falhas de segmento (crashes) no subprocesso não afetam o processo principal Go.
   - *Contras:* Adiciona o binário `ffmpeg` à imagem Docker do worker/API (~30-50MB); overhead mínimo de inicialização de subprocesso (~10-30ms).

---

## 3. Decisão
Adotar a **Opção C (Execução de Subprocesso CLI do `ffmpeg`)**.

O adaptador `FFmpegAudioConverter` implementará a interface `port.Converter` executando o binário `ffmpeg` via `os/exec.CommandContext` dentro do container da aplicação. O fluxo utilizará pipes de entrada/saída (`stdin` / `stdout`) ou arquivos temporários isolados em diretório de trabalho de curta duração (`tmpfs`), garantindo o encerramento do processo em caso de timeout de 60 segundos.

---

## 4. Consequências

* **Positivas:**
  - Garantia de suporte completo e estável para decodificação e codificação `.wav` e `.mp3`.
  - Isolamento estrito: se o `ffmpeg` falhar ou encontrar um arquivo corrompido, apenas o subprocesso é finalizado, permitindo capturar o `stderr` e retornar o erro `CORRUPTED_FILE` sem impactar o servidor HTTP/Worker.
  - Alinhamento total com a premissa de que todas as dependências nativas ficam isoladas dentro do container.

* **Negativas / Mitigações:**
  - *Tamanho da Imagem Docker:* A imagem base do container precisará incluir o pacote `ffmpeg`. (Mitigação: utilizar imagens Alpine/Debian-slim otimizadas).
  - *Gerenciamento de Processos Zumbis:* Garantir que a execução via Go utilize `exec.CommandContext` com encerramento forçado do processo filho em caso de cancelamento do contexto.
