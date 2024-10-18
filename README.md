# GPT Bot

This project implements a GPT-powered bot for Fediverse platforms, currently specifically designed to work with GoToSocial instances. The bot can respond to mentions and interact with users using OpenAI-API compatible generative models.

## Features

- Supports image attachments in conversations
- Responds in same language & visibility & CW & interaction policies as the user's post
- Configurable thread context length & depth
- Different models for local and remote users

## Configuration

The bot is configured using environment variables. You can set these in a `.env` file in the project root. An example configuration is provided in `.env.example`

## Building and Running

### Local Development

1. Clone the repository
2. Copy `.env.example` to `.env` and fill in your configuration
3. Run `go mod download` to install dependencies
4. Build the project with `go build -o gpt-bot`
5. Run the bot with `./gpt-bot`

### Docker Deployment

A Dockerfile is provided for containerized deployment:

To build and run the Docker container:

1. Build the image: `docker build -t gpt-bot .`
2. Run the container: `docker compose up -d`

## Usage

Once the bot is running and connected to your Fediverse instance, it will automatically polling notifications and respond to mentions. The bot processes the conversation history and generates responses using the configured GPT model.

## License

AGPL-3.0
