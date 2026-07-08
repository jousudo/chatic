# ==========================================
# Estágio 1: Compilação Estática
# ==========================================
FROM golang:1.26-alpine AS builder

# Driver SQLite agora é puro-Go (modernc via glebarez) — não precisa de toolchain C.
RUN apk add --no-cache git

WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compilação estática segura (CGO desabilitado, sem símbolos de debug)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o chatic ./cmd/server

# ==========================================
# Estágio 2: Ambiente de Execução Sandboxed
# ==========================================
FROM alpine:latest

# Instala apenas ca-certificates para chamadas HTTPS de nuvem e ffmpeg para mídia
RUN apk add --no-cache ffmpeg ca-certificates

# Cria grupo e usuário não-root dedicados para a execução segura (Sem privilégios)
RUN addgroup -S tutorgroup && adduser -S tutoruser -G tutorgroup

WORKDIR /app

# Copia apenas o binário estático do estágio anterior
COPY --from=builder /build/chatic /app/chatic

# Cria pasta de armazenamento e ajusta as permissões de acesso do usuário não-root
RUN mkdir -p /app/storage && chown -R tutoruser:tutorgroup /app

# Roda o bot exclusivamente como usuário sem privilégios
USER tutoruser

VOLUME [ "/app/storage" ]

EXPOSE 3030

ENTRYPOINT ["/app/chatic"]
