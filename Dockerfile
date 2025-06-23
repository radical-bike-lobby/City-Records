FROM golang:1.23.0

# Set destination for COPY
WORKDIR /app

# Download Go modules
COPY go.mod go.sum ./
RUN go mod download

COPY *.go ./

# Build
RUN CGO_ENABLED=0 GOOS=linux go build -o /city-records

EXPOSE 8080

# Run
CMD ["/city-records"]
