## TODO: Deploymtn automatic CI/CD for github workflow

# First stage to compile the go code
FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# CGO_ENABLED: Complie to static binary file
# GOOS=linux: System
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

# Second stage: runtime image
FROM alpine:3.21
LABEL authors="Rowan"

# Add certification for Postgre connection
# ca-certificates: https://support.circleci.com/hc/en-us/articles/360016505753-Resolve-Certificate-Signed-By-Unknown-Authority-error-in-Alpine-images | For Postgre connection
# tzdat: https://github.com/takecy/tz-sample | For timezone

RUN apk add --no-cache ca-certificates tzdata

# Create a non-root user for security
RUN adduser -D -g '' appuser

WORKDIR /home/appuser/
COPY --from=builder /app/main .
RUN chown appuser:appuser ./main

USER appuser

EXPOSE 8080
ENTRYPOINT ["./main"]