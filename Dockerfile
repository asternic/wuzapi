FROM golang:1.21-alpine AS build
RUN mkdir /app
COPY . /app
WORKDIR /app
RUN go build -o server .

FROM alpine:latest
RUN mkdir /app
COPY ./static /app/static
COPY --from=build /app/server /app/
VOLUME [ "/app/dbdata", "/app/files" ]
WORKDIR /app
CMD [ "/app/server", "-logtype", "json" ]
