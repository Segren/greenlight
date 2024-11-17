FROM golang:1.23-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

# Устанавливаем bash и migrate
RUN apk add --no-cache bash curl \
    && curl -L https://github.com/golang-migrate/migrate/releases/download/v4.15.2/migrate.linux-amd64.tar.gz | tar -xz -C /usr/local/bin \
    && chmod +x /usr/local/bin/migrate

COPY . .

# Устанавливаем переменные окружения для сборки
ARG BUILD_TIME
ARG VERSION
ENV CGO_ENABLED=0

# Собираем проект
RUN go build -o /app/bin/api ./cmd/api

FROM alpine:3.18

WORKDIR /app

# Устанавливаем зависимости для работы приложения и миграций
RUN apk add --no-cache bash curl

# Копируем приложение и утилиту migrate из builder
COPY --from=builder /app/bin/api /app/api
COPY --from=builder /usr/local/bin/migrate /usr/local/bin/migrate
COPY ./migrations ./migrations


# Устанавливаем переменные окружения
ENV GO_ENV=production

# Порт, который будет прослушивать контейнер
EXPOSE 8080

# Команда запуска приложения
CMD migrate -path ./migrations -database "${GREENLIGHT_DB_DSN}" up && /app/api