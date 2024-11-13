FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

COPY . .

# Устанавливаем переменные окружения для сборки
ARG BUILD_TIME
ARG VERSION
ENV CGO_ENABLED=0

# Собираем проект
RUN go build -o /app/bin/api ./cmd/api

FROM alpine:3.18

WORKDIR /app

COPY --from=builder /app/bin/api /app/api

# Устанавливаем переменные окружения
ENV GO_ENV=production

# Порт, который будет прослушивать контейнер
EXPOSE 8080

# Команда запуска приложения
CMD ["/app/api"]