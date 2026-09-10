# Requisitos de Observabilidade — FileConvert API

## 1. Visão Geral e Objetivo

Como o processamento de conversão de arquivos na **FileConvert API** ocorre de forma estritamente assíncrona (com etapas separadas de recepção HTTP, enfileiramento, consumo por workers e escrita em armazenamento binário), a observabilidade é um requisito fundamental de negócio para garantir a transparência, auditabilidade e operacionalidade da plataforma.

O objetivo deste documento é definir **o que** precisa ser observável no ciclo de vida da API e **por que** cada elemento é necessário. 

> [!IMPORTANT]
> **Escopo da Especificação:** Este documento especifica exclusivamente as necessidades funcionais de observabilidade e restrições de negócio. Ele **não prescreve** ferramentas (ex.: Prometheus, Grafana, Jaeger, ELK), nem padrões de instrumentação de código ou bibliotecas de programação.

---

## 2. Rastreabilidade por Job (Audit Trail / Linha do Tempo)

### 2.1. Requisito de Rastreabilidade Integral
Dado um identificador único de job (`job_id`), deve ser possível reconstruir a linha do tempo exata de todas as suas transições de estado, desde a chegada da requisição até a sua resolução final (`done` ou `failed`).

### 2.2. Eventos e Transições Obrigatórias
A linha do tempo de cada job deve conter o registro com carimbo de data/hora (timestamp em UTC com precisão de milissegundos) para as seguintes etapas:

1. **Recepção (`upload`):** 
   - Momento em que o arquivo de origem é recebido pela API HTTP e armazenado no repositório temporário.
2. **Enfileiramento (`pending`):** 
   - Momento em que o `job_id` é registrado e a mensagem de trabalho é publicada na fila.
3. **Início do Processamento (`processing`):** 
   - Momento em que um worker consome o job da fila e inicia a execução da conversão.
4. **Finalização (`done` ou `failed`):** 
   - Momento em que o arquivo convertido é disponibilizado (`done`) ou o erro é capturado e registrado (`failed`).

### 2.3. Justificativa de Negócio
- **Diagnóstico Pós-Mortem:** Identificar a causa exata e o estágio exato onde um job falhou sem necessidade de reprodução manual.
- **Análise de Gargalos de Latência:** Isolar quanto tempo o job permaneceu aguardando na fila (*queue dwell time*) versus quanto tempo o processamento computacional de conversão efetivamente consumiu (*execution time*).
- **Auditoria de SLA:** Validar se a transição total do job respeitou o limite máximo de 60 segundos estipulado no RNF de SLA da v1.

---

## 3. Métricas de Negócio e Operacionais

Para acompanhar a saúde do serviço e o comportamento das conversões em produção, as seguintes métricas de negócio devem ser continuamente geradas:

### 3.1. Taxa de Sucesso e Falha por Par de Formato
- **O que observar:** Quantidade total e proporção percentual de conversões concluídas com sucesso (`done`) e com falha (`failed`), agrupadas por par de formato de conversão (ex.: `wav`→`mp3`, `mp3`→`wav`, `jpg`→`pdf`, `png`→`pdf`, `pdf`→`jpg`, `doc`→`pdf`, `docx`→`pdf`).
- **Por que observar:** Conversores binários (como codecs de áudio ou parsers de documentos) podem apresentar incompatibilidades ou degradações específicas em determinados par de formatos sem afetar os demais. Essa segmentação evita que falhas em um formato específico fiquem ocultas na média geral de erros da API.

### 3.2. Tempo Médio e Percentil 95 (p95) de Conversão
- **O que observar:** Duração do tempo de processamento ativo do conversor (em milissegundos e segundos), discriminado por par de formato de conversão, acompanhando a média e o percentil 95 (p95).
- **Por que observar:** O p95 representa a experiência dos 5% de jobs mais lentos e permite identificar antecipadamente se conversões mais pesadas (ex.: `.docx` complexos ou áudios longos) estão se aproximando do limite de timeout de 60 segundos, permitindo atuação preventiva antes da violação do SLA.

### 3.3. Tamanho e Retenção da Fila de Jobs Pendentes
- **O que observar:** 
  - Quantidade atual de jobs no estado `pending` aguardando processamento.
  - Taxa de entrada de novos jobs (enfileiramento) versus taxa de saída (desenfileiramento por workers).
  - Tempo de permanência do job na fila antes do início da conversão.
- **Por que observar:** O acúmulo de jobs na fila é o principal indicador de sobrecarga ou indisponibilidade de workers. Essa métrica fornece o subsídio necessário para decisões de dimensionamento de capacidade (autoscaling de workers).

### 3.4. Volume de Requisições e Rejeições na Borda
- **O que observar:** Total de requisições de conversão recebidas, aceitas (geraram `job_id`) e rejeitadas síncronamente (erros de validação como `EMPTY_FILE`, `FILE_TOO_LARGE`, `UNSUPPORTED_FORMAT`).
- **Por que observar:** Distinguir problemas de uso da API (erros de entrada de clientes) de falhas operacionais do sistema interno.

---

## 4. Critérios de Alerta ("Algo está errado")

Um estado de anomalia operacional é caracterizado por desvios nos indicadores de negócio ou incapacidade de vazão dos workers. O sistema de observabilidade deve fornecer condições claras para acionamento de alertas com base nos seguintes critérios:

### 4.1. Alerta: Taxa de Falha Elevada por Par de Formato
- **Condição Anômala:** A taxa de falhas de conversão para um par de formatos específico (ex.: `docx`→`pdf`) ultrapassar **5%** das solicitações em uma janela deslizante de **5 minutos** (ou > 10% no volume total global).
- **Motivo de Negócio:** Sinaliza quebra no conversor subjacente, corrupção em bibliotecas de terceiros ou incompatibilidade crítica introduzida em um formato específico.

### 4.2. Alerta: Fila Represada / Paralisação de Workers
- **Condição Anômala:** O tamanho da fila de jobs pendentes crescer continuamente por mais de **3 minutos** sem redução, ou a taxa de desenfileiramento por workers ser igual a zero enquanto houver jobs `pending`.
- **Motivo de Negócio:** Indica que todos os workers estão inoperantes, travados em deadlock ou inacessíveis, interrompendo a capacidade da API de cumprir seu papel assíncrono.

### 4.3. Alerta: Estouro Recorrente de SLA (Jobs em Timeout)
- **Condição Anômala:** Ocorrência de **3 ou mais jobs** finalizando no estado `failed` com o código `JOB_TIMEOUT` (excedendo o limite de 60s) dentro de um intervalo de **10 minutos**.
- **Motivo de Negócio:** Indica gargalo severo de CPU/memória nos workers ou envio recorrente de arquivos excessivamente complexos que travam a esteira.

### 4.4. Alerta: Pico de Rejeição por Erro de Entrada na Borda
- **Condição Anômala:** Mais de **20%** das requisições HTTP de entrada sendo rejeitadas síncronamente por erros de validação (`EMPTY_FILE`, `FILE_TOO_LARGE`, `UNSUPPORTED_FORMAT`) em um janela de 5 minutos.
- **Motivo de Negócio:** Alerta para possíveis ataques de negação de serviço (DoS), integrações defeituosas nos clientes ou falhas em formulários de upload de clientes.

---

## 5. Restrição de Privacidade, Segurança e Proteção de Dados

> [!CAUTION]
> **REGRAS ABSOLUTAS DE PRIVACIDADE:**
> O conteúdo dos arquivos dos usuários é estritamente confidencial e NUNCA pode ser exposto em logs, traces ou métricas.

### 5.1. O que NUNCA pode ser coletado ou registrado
- **Conteúdo Binário do Arquivo:** Nenhum byte, trecho de código ou dump binário do arquivo de origem ou de destino.
- **Texto Extraído / Transcrição:** Conteúdo textual interno de documentos (`.doc`, `.docx`, `.pdf`) ou transcrições de áudio (`.wav`, `.mp3`).
- **Nomes de Arquivos Originais:** Nomes de arquivos enviados pelos usuários (ex.: `demonstrativo_financeiro_2026.pdf` ou `rg_cliente.jpg`) não podem ser registrados diretamente em logs ou rótulos de métricas, pois frequentemente contêm Dados Pessoais Identificáveis (PII) ou informações confidenciais.
- **Dados Pessoais Identificáveis (PII):** CPFs, nomes, e-mails, endereços ou qualquer informação pessoal presente no documento.

### 5.2. O que É PERMITIDO registrar (Metadados Operacionais)
Apenas metadados puramente técnicos e estruturais necessários para o funcionamento e diagnóstico da API podem ser coletados:

| Categoria | Metadados Permitidos | Exemplo / Limite |
| :--- | :--- | :--- |
| **Identificadores** | `job_id` gerado pelo sistema | `job_123456789` |
| **Formatos** | Formato de origem (`source_format`) e formato de destino (`target_format`) | `wav`, `mp3`, `pdf`, `docx` |
| **Tamanho** | Tamanho do arquivo em bytes | `1048576` (1 MB) |
| **Duração** | Latência das etapas de processamento em ms | `450ms` |
| **Erros** | Código textual do erro padronizado e classe da falha | `CORRUPTED_FILE`, `JOB_TIMEOUT` |
| **Status** | Estado no ciclo de vida do job | `pending`, `processing`, `done`, `failed` |

### 5.3. Justificativa de Negócio
Garantir total conformidade com legislações de privacidade (LGPD, GDPR), proteger a propriedade intelectual dos clientes e eliminar o risco de vazamento acidental de dados sensíveis em sistemas externos de monitoramento.

---

## 6. Matriz de Resumo dos Requisitos de Observabilidade

| Requisito de Observabilidade | Tipo | Estágio Principal | Motivo Primário de Negócio |
| :--- | :--- | :--- | :--- |
| **Linha do Tempo por `job_id`** | Rastreabilidade | Ciclo Completo | Auditoria de SLA (60s) e diagnóstico pós-mortem. |
| **Métricas por Par de Formato** | Métrica | Processamento | Detectar degradação específica de codecs/parsers. |
| **Monitoramento da Fila** | Métrica | Enfileiramento | Autoscaling e detecção de retenção excessiva. |
| **Alertas de Falha e Timeout** | Alertas | Execução / Erro | Notificação proativa de quebra de serviço. |
| **Sanitização de Privacidade** | Restrição | Coleta Global | Conformidade legal (LGPD) e sigilo do cliente. |
