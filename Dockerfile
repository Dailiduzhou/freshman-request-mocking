# 多阶段构建：静态编译，scratch 运行，无 shell、最小攻击面。
FROM golang:1.27.2-alpine3.24 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM scratch
COPY --from=build /out/server /server
EXPOSE 8080
ENTRYPOINT ["/server"]