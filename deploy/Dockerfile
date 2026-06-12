FROM golang:1.25.0-alpine as build
MAINTAINER luankz
WORKDIR /app
COPY . .
RUN go mod tidy
ARG VERSION=unknown
RUN go build -ldflags="-X main.version=${VERSION} -w -s" -a -o dist/eflow main.go
FROM chromedp/headless-shell:133.0.6905.0 as deploy
RUN apt-get update && apt-get install -y ca-certificates fonts-wqy-microhei && update-ca-certificates
ENV EGO_DEBUG=true
ENV PATH /headless-shell:$PATH
WORKDIR /app
COPY --from=build /app/dist/eflow .
EXPOSE 8000
ENTRYPOINT ["./eflow", "start"]
