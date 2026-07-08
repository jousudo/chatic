#!/bin/bash
# Copyright (c) 2026 Chatic Contributors
# Licensed under the Apache License, Version 2.0. See LICENSE in the project root for license information.
#
# Build local multiplataforma dos pacotes de distribuição (binários, .deb, .rpm,
# archives) usando o GoReleaser em modo snapshot — NÃO publica nada no GitHub.
# Os artefatos ficam em ./dist. Para publicar de verdade, use uma git tag + CI
# (.github/workflows/release.yml) ou `goreleaser release --clean`.
set -e

echo "🔒 Gerando pacotes de distribuição (snapshot local, sem publicar)…"

# Localiza o goreleaser (PATH ou GOPATH/bin).
GORELEASER="$(command -v goreleaser || true)"
if [ -z "$GORELEASER" ] && [ -x "$(go env GOPATH)/bin/goreleaser" ]; then
    GORELEASER="$(go env GOPATH)/bin/goreleaser"
fi

if [ -z "$GORELEASER" ]; then
    echo "❌ GoReleaser não encontrado. Instale com:"
    echo "   go install github.com/goreleaser/goreleaser/v2@latest"
    echo "   (e garanta que \$(go env GOPATH)/bin está no PATH)"
    exit 1
fi

echo "🔍 Validando .goreleaser.yaml…"
"$GORELEASER" check

echo "🏗️  Compilando e empacotando…"
"$GORELEASER" release --snapshot --clean

echo "--------------------------------------------------------"
echo "🎉 Artefatos gerados em ./dist:"
ls -1 dist/*.tar.gz dist/*.zip dist/*.deb dist/*.rpm 2>/dev/null || true
echo "--------------------------------------------------------"
