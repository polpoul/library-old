# ── Stage 1 : compilation ────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
COPY main.go ./

# Binaire statique (pas de libc requise → image scratch possible)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o library .

# ── Stage 2 : image finale minimale ──────────────────────────────────────────
FROM alpine:3.19

# Certificats TLS (utile si le service fait des appels HTTPS sortants)
RUN apk add --no-cache ca-certificates

WORKDIR /app

# Binaire compilé
COPY --from=builder /app/library .

# Fichiers statiques du frontend
COPY public/ ./public/

# Le dossier ressources sera monté en volume depuis le VPS
# On crée le point de montage pour éviter une erreur si non monté
RUN mkdir -p ./ressources

EXPOSE 3000

ENV PORT=3000
ENV RESSOURCES_PATH=/app/ressources

CMD ["./library"]
