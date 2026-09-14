#!/usr/bin/env bash
# ==============================================================================
# Cloud-Init Provisioning Script - Oracle Cloud Infrastructure (OCI)
# FileConvert API - Infraestrutura de Produção Segura
# ==============================================================================

set -euo pipefail

DOMAIN="${DOMAIN:-api.fileconvert.example.com}"
ADMIN_EMAIL="${ADMIN_EMAIL:-admin@fileconvert.example.com}"
# IP RESTRIÇÃO DE SSH: Altere para o seu IP público fixo de administração (NUNCA utilize 0.0.0.0/0)
ADMIN_SSH_IP="${ADMIN_SSH_IP:-203.0.113.15/32}"
APP_DIR="/opt/file-convert-api"

echo "[1/6] Atualizando pacotes do sistema..."
export DEBIAN_FRONTEND=noninteractive
apt-get update && apt-get upgrade -y
apt-get install -y curl git ufw iptables-persistent certbot cron

echo "[2/6] Instalando Docker Engine e Docker Compose Plugin..."
if ! command -v docker &> /dev/null; then
    curl -fsSL https://get.docker.com | sh
    systemctl enable --now docker
    usermod -aG docker ubuntu || true
fi

echo "[3/6] Configurando Firewall do Host (iptables/UFW) alinhado ao OCI Security List..."
# Resetar regras de firewall do sistema
iptables -F INPUT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT

# 1. Liberar HTTP (Porta 80) e HTTPS (Porta 443) para a internet (0.0.0.0/0)
iptables -A INPUT -p tcp --dport 80 -j ACCEPT
iptables -A INPUT -p tcp --dport 443 -j ACCEPT

# 2. RESTRIÇÃO DE SEGURANÇA SSH (Porta 22): Permitir unicamente o IP administrativo autorizado
iptables -A INPUT -p tcp --dport 22 -s "$ADMIN_SSH_IP" -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -j DROP # Bloqueio rígido de SSH para 0.0.0.0/0

# Salvar regras de iptables persistentemente
netfilter-persistent save

echo "[4/6] Configurando diretórios de aplicação e certificados TLS..."
mkdir -p "$APP_DIR"
mkdir -p /var/www/certbot
mkdir -p "$APP_DIR/deploy/observability/grafana/provisioning/datasources"
mkdir -p "$APP_DIR/deploy/observability/grafana/provisioning/dashboards"

# Se o certificado ainda não existir, obter via Certbot em modo standalone inicial
if [ ! -d "/etc/letsencrypt/live/$DOMAIN" ]; then
    echo "Obtendo certificado TLS inicial via Let's Encrypt para $DOMAIN..."
    certbot certonly --standalone \
        --non-interactive \
        --agree-tos \
        --email "$ADMIN_EMAIL" \
        -d "$DOMAIN" || echo "AVISO: Certbot falhou na primeira tentativa; valide a propagação do DNS."
fi

echo "[5/6] Configurando Renovação Automática do Certificado TLS (Certbot Auto-Renew Cron)..."
cat << 'EOF' > /etc/cron.d/certbot-renewal
# Renovação automática diária às 03:00 AM com reload do Nginx
0 3 * * * root certbot renew --quiet --post-hook "docker exec fileconvert-nginx nginx -s reload"
EOF
chmod 0644 /etc/cron.d/certbot-renewal

echo "[6/6] Subindo a stack de containers da FileConvert API..."
if [ -f "$APP_DIR/docker-compose.yml" ]; then
    cd "$APP_DIR"
    docker compose up -d --build
fi

echo "=============================================================================="
echo "Provisionamento OCI concluído com sucesso!"
echo "Domínio: https://$DOMAIN"
echo "SSH Restrito ao IP: $ADMIN_SSH_IP (Porta 22 0.0.0.0/0 BLOQUEADA)"
echo "=============================================================================="
