FROM alpine:latest

# git builds the packages of pack resources
RUN apk add --no-cache tzdata git

ENV TZ=Asia/Shanghai

WORKDIR /app

COPY bin/app .

EXPOSE 5432

ENTRYPOINT ["./app"]