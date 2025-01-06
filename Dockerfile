FROM ubuntu:latest

WORKDIR /app

COPY resource-backend resource-backend

EXPOSE 5432

ENTRYPOINT ["./resource-backend"]