FROM golang:1.25.0-alpine as build
MAINTAINER luankz
WORKDIR /app
COPY . .
# ENV GOPROXY="https://goproxy.cn,direct"
# ENV GOOS=linux GOARCH=amd64 CGO_ENABLED=0
RUN go mod tidy

ARG SERVICE_NAME=eflow
RUN go build -a -o dist/${SERVICE_NAME} -ldflags="-w -s" .

FROM alpine:3.21 as deploy
WORKDIR /app
ENV EGO_DEBUG=true
ARG SERVICE_NAME=eflow
COPY --from=build /app/dist/${SERVICE_NAME} .
EXPOSE 8766
