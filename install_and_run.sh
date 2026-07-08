#!/bin/bash
# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.

# Script de Instalação e Execução com Isolamento do Sistema Operacional (Sandboxing)
#
# ⚠️  DESCONTINUADO — prefira os métodos oficiais de instalação:
#   • Linux/macOS: curl -fsSL .../install.sh | sh
#   • Windows:     irm .../install.ps1 | iex
#   • Linux com serviço + auto-start: pacote .deb/.rpm (systemd)
#   • Docker: imagem em ghcr.io/jousudo/chatic
# Este script antigo é mantido apenas para referência.

set -e

echo "⚠️  install_and_run.sh está descontinuado. Veja install.sh / pacotes .deb/.rpm / Docker."
echo "🚀 Iniciando Instalador Rápido e Seguro do Tutor de Idiomas..."

# 1. Detecção automática de Plataforma (OS e Arquitetura)
OS="$(uname -s)"
ARCH="$(uname -m)"
BUNDLE_FILE=""
EXTRACT_CMD=""

case "$OS" in
    Linux)
        echo "💻 Sistema detectado: Linux ($ARCH)"
        BUNDLE_FILE="tutor-idiomas-linux-amd64.tar.gz"
        EXTRACT_CMD="tar -xzf"
        ;;
    Darwin)
        if [ "$ARCH" = "arm64" ]; then
            echo "🍏 Sistema detectado: macOS Apple Silicon ($ARCH)"
            BUNDLE_FILE="tutor-idiomas-macos-arm64.tar.gz"
        else
            echo "🍏 Sistema detectado: macOS Intel ($ARCH)"
            BUNDLE_FILE="tutor-idiomas-macos-amd64.tar.gz"
        fi
        EXTRACT_CMD="tar -xzf"
        ;;
    MSYS*|MINGW*|CYGWIN*)
        echo "🔌 Sistema detectado: Windows ($ARCH)"
        BUNDLE_FILE="tutor-idiomas-windows-amd64.zip"
        EXTRACT_CMD="unzip -q"
        ;;
    *)
        echo "❌ Sistema operacional não suportado: $OS"
        exit 1
        ;;
esac

# 2. Download do Bundle
RELEASE_URL="${TUTOR_RELEASE_URL:-https://raw.githubusercontent.com/chatic/tutor-idiomas/main/releases}"

echo "🔹 Verificando arquivo do pacote..."
if [ -f "$BUNDLE_FILE" ]; then
    echo "📦 Pacote local '$BUNDLE_FILE' encontrado. Utilizando arquivo local..."
else
    DOWNLOAD_URL="$RELEASE_URL/$BUNDLE_FILE"
    echo "📥 Baixando pacote de: $DOWNLOAD_URL"
    if command -v curl &> /dev/null; then
        curl -fsSL -O "$DOWNLOAD_URL"
    elif command -v wget &> /dev/null; then
        wget -q "$DOWNLOAD_URL"
    else
        echo "❌ Erro: Instale 'curl' ou 'wget' para realizar o download."
        exit 1
    fi
fi

# 3. Extração
echo "🔹 Extraindo arquivos do bundle..."
$EXTRACT_CMD "$BUNDLE_FILE"

# 4. Inicialização de Configuração (.env básico sem credenciais em texto plano)
if [ ! -f .env ]; then
    echo "🔹 Criando arquivo de ambiente básico (.env)..."
    cat <<EOT > .env
PORT=3030
ENV=production
PRIMARY_LLM_PROVIDER=gemini
DATABASE_PATH=storage/tutor.db
INITIAL_ADMIN_NUMBER=
EOT
fi

mkdir -p storage/tts

# 5. Configuração de Confinamento (Apenas Linux)
if [ "$OS" = "Linux" ]; then
    # Ativação do AppArmor se disponível e se rodando como Root
    if [ "$EUID" -eq 0 ] && command -v apparmor_parser &> /dev/null && [ -f chatic.apparmor ]; then
        echo "🛡️ Ativando perfil AppArmor de segurança extrema para o bot..."
        cp chatic.apparmor /etc/apparmor.d/home.jou.ingles.chatic
        apparmor_parser -r -W /etc/apparmor.d/home.jou.ingles.chatic || echo "Aviso: Falha ao carregar perfil AppArmor. Continuando..."
    fi

    # Instalação do Serviço Systemd Hardened se rodando como Root
    if [ "$EUID" -eq 0 ]; then
        echo "⚙️ Configurando serviço Systemd enjaulado (Sandboxing)..."
        
        # Garante a existência do usuário sem privilégios tutoruser
        id -u tutoruser &>/dev/null || useradd -r -s /bin/false tutoruser
        chown -R tutoruser:tutoruser storage

        cat <<EOT > /etc/systemd/system/chatic.service
[Unit]
Description=Chatic — WhatsApp Language Tutor
After=network.target

[Service]
ExecStart=$(pwd)/chatic
WorkingDirectory=$(pwd)
Restart=always
User=tutoruser
Group=tutoruser

# --- DIRETIVAS DE CONFINAMENTO SYSTEMD ---
ProtectSystem=strict
ProtectHome=true
PrivateTmp=true
NoNewPrivileges=true
ReadWritePaths=$(pwd)/storage
CapabilityBoundingSet=

[Install]
WantedBy=multi-user.target
EOT

        systemctl daemon-reload
        systemctl enable chatic
        systemctl start chatic
        echo "✔ Serviço systemd 'chatic' iniciado com sandboxing ativado."
        RUNNING_SERVICE=true
    fi
fi

# 6. Execução Manual se não instalado como serviço Systemd
if [ "$RUNNING_SERVICE" != "true" ]; then
    echo "🔹 Iniciando o Chatic em segundo plano..."
    BIN_EXEC="./chatic"
    if [ "$OS" = "WindowsNT" ] || [[ "$OS" == MSYS* ]] || [[ "$OS" == MINGW* ]]; then
        BIN_EXEC="./chatic.exe"
    fi
    nohup $BIN_EXEC > bot_runtime.log 2>&1 &
    BOT_PID=$!
    echo "✔ Bot iniciado com PID $BOT_PID. Logs em 'bot_runtime.log'."
fi

echo "⏳ Aguardando inicialização do servidor HTTP (porta 3030)..."
sleep 2

# 7. Abertura da interface pública inicial
PUBLIC_URL="http://localhost:3030/"
echo "🌐 Abrindo página inicial em: $PUBLIC_URL"

if command -v open &> /dev/null; then
    open "$PUBLIC_URL"
elif command -v xdg-open &> /dev/null; then
    xdg-open "$PUBLIC_URL"
elif command -v sensible-browser &> /dev/null; then
    sensible-browser "$PUBLIC_URL"
else
    cmd.exe /c start "$PUBLIC_URL" 2>/dev/null || echo "💡 Por favor, acesse manualmente $PUBLIC_URL no seu navegador para configurar."
fi

echo "--------------------------------------------------------"
echo "🎉 INSTALAÇÃO E EXECUÇÃO CONCLUÍDAS COM SUCESSO!"
echo "--------------------------------------------------------"
echo "🔐 Acesse a página administrativa sob '/admin' e crie"
echo "   sua conta mestra no setup inicial de primeiro acesso."
echo "--------------------------------------------------------"
