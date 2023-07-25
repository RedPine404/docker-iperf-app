# Start from the official Go base image
FROM golang:1.17-alpine AS builder

# Set the working directory
WORKDIR /app

# Copy the source code into the container
COPY . .

# Build the application
RUN go build -o app .

# Start a new stage with a minimal image
FROM alpine:latest

# Install iperf3
RUN apk add --no-cache iperf3

# Set the working directory
WORKDIR /app

# Copy the compiled executable from the builder stage
COPY --from=builder /app/app /app/app
COPY config.json /app/config.json

# Expose the port on which the application listens
EXPOSE 8000

# Run the application
CMD ["./app"]
