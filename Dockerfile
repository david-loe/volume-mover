FROM node:22-alpine AS webbuild
WORKDIR /src
COPY webapp/package*.json webapp/
WORKDIR /src/webapp
RUN npm ci
WORKDIR /src
COPY . .
WORKDIR /src/webapp
RUN npm run build

FROM golang:1.22-alpine AS build
RUN apk add --no-cache gcc musl-dev
WORKDIR /src
COPY --from=webbuild /src /src
RUN go build -o /out/volume-mover ./cmd/volume-mover

FROM alpine:3.20
RUN apk add --no-cache bash ca-certificates openssh-client docker-cli
COPY --from=build /out/volume-mover /usr/local/bin/volume-mover
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/volume-mover"]
CMD ["web", "--listen", "0.0.0.0:8080"]
