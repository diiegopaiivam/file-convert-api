# Notas de Implantação e Segurança de Infraestrutura OCI — FileConvert API

## 1. Visão Geral da Infraestrutura na Oracle Cloud Infrastructure (OCI)

A **FileConvert API** é implantada em uma instância de Máquina Virtual (VM) pública na Oracle Cloud Infrastructure (OCI), protegida por um modelo de segurança em camadas composto por:
1. **OCI VCN Security List (Firewall de Rede da Nuvem)**
2. **Iptables Host Firewall (Firewall do Sistema Operacional)**
3. **Reverse Proxy Nginx com TLS 1.2/1.3 e Hardening de Cabeçalhos HTTP**
4. **Certbot (Let's Encrypt) com Renovação Automática**

---

## 2. Configuração de Rede e Security List da OCI

A Virtual Cloud Network (VCN) e a Subnet pública da OCI devem ser configuradas estritamente com as seguintes regras de firewall:

### 2.1. Regras de Entrada (Ingress Security Rules)

| Porta / Protocolo | Origem (CIDR) | Finalidade / Justificativa | Estado |
| :--- | :--- | :--- | :--- |
| **TCP 80** | `0.0.0.0/0` | Tráfego HTTP público e validação de desafio ACME do Certbot (redirecionado para 443). | **PERMITIDO** |
| **TCP 443** | `0.0.0.0/0` | Tráfego HTTPS público da API com criptografia TLS 1.2/1.3. | **PERMITIDO** |
| **TCP 22** | **`<IP_ADMIN_FIXO>/32`** | Acesso de administração remota via SSH unicamente para o IP do administrador. | **PERMITIDO (RESTRITO)** |
| **TCP 22** | **`0.0.0.0/0`** | Acesso SSH aberto para a internet global. | **PROIBIDO (BLOQUEADO)** |

> [!CRITICAL]
> **NUNCA liberte a porta 22 (SSH) para `0.0.0.0/0`.** O acesso SSH deve obrigatoriamente ser restrito ao CIDR `/32` do IP fixo da equipe de infraestrutura para mitigar ataques de força bruta.

### 2.2. Regras de Saída (Egress Security Rules)

| Porta / Protocolo | Destino (CIDR) | Finalidade | Estado |
| :--- | :--- | :--- | :--- |
| **Todas as Portas / IP** | `0.0.0.0/0` | Atualização de pacotes do SO, validação do Let's Encrypt e métricas externas. | **PERMITIDO** |

---

## 3. Configuração de TLS / SSL e Renovação Automática (Certbot)

A comunicação externa é obrigatoriamente criptografada com certificados digitais válidos emitidos pela autoridade certficadora **Let's Encrypt**.

### 3.1. Obtenção do Certificado TLS Inicial
O script de provisionamento [deploy/oracle-cloud/cloud-init.sh](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/oracle-cloud/cloud-init.sh) executa o Certbot em modo standalone para gerar os certificados em `/etc/letsencrypt/live/api.fileconvert.example.com/`:
- `fullchain.pem`: Certificado X.509 da API + cadeia intermediária do Let's Encrypt.
- `privkey.pem`: Chave privada TLS de 2048/4096 bits.

### 3.2. Renovação Automática via Cron
Para evitar expiração do certificado (validade de 90 dias do Let's Encrypt), o arquivo `/etc/cron.d/certbot-renewal` executa diariamente às 03:00 AM:

```cron
0 3 * * * root certbot renew --quiet --post-hook "docker exec fileconvert-nginx nginx -s reload"
```

Quando um certificado for renovado, o hook `--post-hook` recarrega a configuração do container Nginx sem causar *downtime*.

---

## 4. Hardening do Nginx e Cabeçalhos de Segurança HTTP

O arquivo de configuração [deploy/oracle-cloud/nginx.conf](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/oracle-cloud/nginx.conf) implementa boas práticas OWASP de segurança de infraestrutura:

### 4.1. Cabeçalhos de Segurança Injetados

1. **`Strict-Transport-Security` (HSTS):**
   ```nginx
   add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
   ```
   *Efeito:* Força os navegadores e clientes HTTP a utilizarem exclusivamente HTTPS por 2 anos.

2. **`X-Content-Type-Options`:**
   ```nginx
   add_header X-Content-Type-Options "nosniff" always;
   ```
   *Efeito:* Impede a interpretação (*MIME sniffing*) de arquivos enviados como executáveis ou scripts.

3. **`X-Frame-Options`:**
   ```nginx
   add_header X-Frame-Options "DENY" always;
   ```
   *Efeito:* Protege contra ataques de *Clickjacking*, proibindo a renderização da API dentro de `<iframe>` ou `<frame>`.

4. **`X-XSS-Protection` & `Content-Security-Policy`:**
   ```nginx
   add_header X-XSS-Protection "1; mode=block" always;
   add_header Content-Security-Policy "default-src 'none'; frame-ancestors 'none';" always;
   ```
   *Efeito:* Bloqueia execução de scripts não autorizados e proíbe incorporação por terceiros.

5. **Limite de Tamanho de Payload (`client_max_body_size`):**
   ```nginx
   client_max_body_size 50M;
   ```
   *Efeito:* Alinhado ao **ADR 0006**, rejeita requisições com corpo superior a 50 MB antes de sobrecarregar a memória do container API ou do Worker.

---

## 5. Script de Provisionamento Automático (`cloud-init.sh`)

O arquivo [deploy/oracle-cloud/cloud-init.sh](file:///Users/digopaiva/Documents/projects/file-convert-api/deploy/oracle-cloud/cloud-init.sh) é injetado como script de inicialização do ecossistema ao criar a VM na Oracle Cloud.

### 5.1. Etapas Executadas pelo Script:
1. Atualiza os repositórios `apt` do Ubuntu/Debian e instala Docker, Docker Compose e Certbot.
2. Limpa regras padrão restritivas de firewall do Ubuntu e reconfigura o `iptables`:
   - Libera portas `80` e `443` para `0.0.0.0/0`.
   - **Bloqueia a porta `22` (SSH) para `0.0.0.0/0`**, liberando apenas o IP administrativo (`$ADMIN_SSH_IP`).
3. Solicita os certificados TLS iniciais do Let's Encrypt.
4. Registra a tarefa cron de renovação diária do certificado TLS.
5. Inicia a stack Docker (`docker compose up -d`).

---

## 6. Procedimento de Implantação Passo a Passo

1. **Criar a Instância VM na OCI:**
   - Shape recomendado: `VM.Standard.A1.Flex` (Ampere ARM 2 vCPU, 8GB RAM).
   - Em **Advanced Options -> User Data**, selecione o arquivo `deploy/oracle-cloud/cloud-init.sh`.
2. **Configurar a VCN Security List na OCI Console:**
   - Adicionar Ingress Rule: TCP 80 (`0.0.0.0/0`)
   - Adicionar Ingress Rule: TCP 443 (`0.0.0.0/0`)
   - Adicionar Ingress Rule: TCP 22 (`SEU_IP_PUBLICO/32`)
3. **Apontamento de DNS:**
   - Crie um registro `A` no DNS apontando `api.fileconvert.example.com` para o IP público reservado da VM OCI.
