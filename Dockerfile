FROM golang:1.25 AS builder

WORKDIR /app

COPY go.mod ./
COPY . ./

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/meme-admin-bot .

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
	&& apt-get install -y --no-install-recommends ca-certificates ffmpeg nodejs python3 python3-pip \
	&& pip3 install --break-system-packages yt-dlp \
	&& rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /out/meme-admin-bot /usr/local/bin/meme-admin-bot

RUN mkdir -p /app/data /app/data/tmp

ENV DATA_DIR=/app/data
ENV TEMP_DIR=/app/data/tmp
ENV YT_DLP_BINARY=yt-dlp
ENV FFMPEG_BINARY=ffmpeg

CMD ["meme-admin-bot"]
