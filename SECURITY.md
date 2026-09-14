# Política de Segurança e Divulgação Responsável — FileConvert API

A equipe da **FileConvert API** leva a sério a segurança cibernética e a privacidade dos dados de nossos usuários. Como uma plataforma *open source* que processa arquivos binários enviadas por terceiros utilizando ferramentas nativas (`ffmpeg`, `LibreOffice`, `ImageMagick` e `Poppler`), reconhecemos a importância da colaboração com a comunidade de segurança para a identificação e correção preventiva de falhas.

---

## 1. Como Reportar uma Vulnerabilidade (Reporte Privado)

> [!CRITICAL]
> **NUNCA crie uma Issue pública no GitHub para reportar uma vulnerabilidade de segurança.**
> Abrir Issues públicas para falhas não corrigidas expõe todos os usuários da aplicação a riscos de exploração maliciosa antes que um patch de correção seja disponibilizado.

### Canais de Reporte Privado:

Para reportar uma vulnerabilidade de forma segura e confidencial, utilize um dos seguintes canais mantidos pela equipe de segurança:

1. **GitHub Private Vulnerability Reporting (Recomendado):**
   - Acesse a aba **Security** do repositório principal no GitHub.
   - Clique em **Report a vulnerability** para abrir um formulário privado de aviso de segurança.
2. **E-mail de Segurança Privado:**
   - Envie os detalhes da vulnerabilidade para o e-mail de segurança do projeto (ex.: `security@fileconvert.example.com` ou o canal oficial de contato mantido pelos mantenedores).
   - Se desejar, utilize nossa chave PGP de segurança para criptografar a mensagem.

### Informações Recomendadas no Reporte:
Para acelerar a triagem e validação, inclua:
- Descrição detalhada da vulnerabilidade e o componente afetado (ex.: API HTTP, Worker, conversor nativo).
- Passos para reprodução (*Proof of Concept* - PoC), incluindo o payload de arquivo binário se aplicável.
- Avaliação de impacto estimado (ex.: RCE, bypass de autenticação, vazamento de PII).

---

## 2. Prazos Esperados e SLA de Resposta

Assumimos o compromisso de responder aos reportes de segurança com agilidade e transparência:

| Etapa do Processo | SLA / Prazo Máximo | Descrição |
| :--- | :--- | :--- |
| **Confirmação de Recebimento** | **Até 48 horas úteis** | Resposta inicial confirmando que a mensagem foi recebida e encaminhada para a equipe técnica. |
| **Triagem e Validação** | **Até 5 dias úteis** | Análise técnica de reprodutibilidade, classificação do impacto e definição do score CVSS. |
| **Publicação do Patch (CRITICAL/HIGH)** | **Até 24 a 48 horas** | Correção de emergência publicada na branch principal e lançamento de nova tag/release. |
| **Publicação do Patch (MEDIUM/LOW)** | **Até 7 a 30 dias** | Correção incorporada no ciclo regular de manutenção e lançamentos. |

Garantimos manter o pesquisador informado sobre o progresso da correção e o agendamento da divulgação pública (*Advisory publication*).

---

## 3. Escopo Elegível de Vulnerabilidades

### 3.1. Dentro do Escopo (*In-Scope*):

Consideramos vulnerabilidades válidas e relevantes neste projeto:
- **Execução Remota de Código (RCE):** Exploração de parsers de arquivos binários nativos (`ffmpeg`, `LibreOffice`, `ImageMagick`, `Poppler`) para execução arbitrária de comandos no worker.
- **Evasão de Isolamento (Container Breakout / Sandbox Escape):** Quebra de contêiner ou desrespeito ao isolamento de subprocesso (conforme [ADR 0005](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0005-isolamento-subprocesso-conversao.md)).
- **Bypass de Autenticação ou Rate Limiting:** Burlar a validação do `AuthProvider` ou ultrapassar limites de requisições e quotas de jobs simultâneos no `RateLimiter` sem a devida permissão (conforme [ADR 0007](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0007-autenticacao-e-rate-limit.md)).
- **Vazamento de Dados e PII:** Armazenamento indevido de conteúdo binário ou PII de arquivos de usuários nos subsistemas de observabilidade (logs, métricas ou tracing), violando o [ADR 0008](file:///Users/digopaiva/Documents/projects/file-convert-api/docs/architecture/adr/0008-retencao-de-dados.md).
- **Exaustão Não Tratada de Recursos:** Ataques de *Zip Bomb* ou de descompressão binária que evadam os limites de recurso (`ResourceLimits`) e afetem a disponibilidade global da VM.

### 3.2. Fora do Escopo (*Out-of-Scope*):

Não consideramos elegíveis no momento:
- Ataques de Negação de Serviço por Volumetria Bruta de Rede (L3/L4 DDoS).
- Vulnerabilidades que dependam de acesso físico direto à máquina virtual ou conta de administração da nuvem (OCI).
- Relatórios gerados por scanners automáticos (ex.: ZAP, Nessus) sem PoC validando reprodutibilidade e impacto real.
- Falhas de *Social Engineering*, *Phishing* ou ataques contra os colaboradores do projeto.

---

## 4. Divulgação Responsável e Agradecimentos (*Hall of Fame*)

Valorizamos imensamente o tempo e o esforço dispensados por pesquisadores independentes que atuam em conformidade com as boas práticas de divulgação responsável (*Responsible Disclosure*).

### Nosso Compromisso com o Pesquisador:
- Não tomaremos medidas judiciais contra pesquisadores que realizem pesquisas de boa-fé em ambiente de testes e reportem as falhas de forma privada.
- Não divulgaremos a vulnerabilidade publicamente antes que a correção esteja pronta e implantada.

### Agradecimento Público (*Hall of Thanks / Hall of Fame*):
Em sinal de reconhecimento e gratidão, pesquisadores que reportarem vulnerabilidades válidas de forma responsável terão seus nomes/handles eternizados no nosso **Hall de Agradecimento de Segurança** nas notas de lançamento (*Release Notes*) do projeto e no arquivo de documentação oficial.

---

*Obrigado por nos ajudar a manter a FileConvert API segura para toda a comunidade!*
