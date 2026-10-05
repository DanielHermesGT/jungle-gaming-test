#!/usr/bin/env bash
# Roda automaticamente quando o LocalStack fica "ready"
# (montado em /etc/localstack/init/ready.d/ via docker-compose).
#
# Cria as 3 filas FIFO do desafio (README §10 / §11):
#   1) wager-transactions-dlq.fifo  — DLQ (mensagens que falharam demais)
#   2) wager-transactions.fifo      — entrada do consumer (apostas via SQS)
#   3) domain-events.fifo           — destino do publisher de outbox
#

# "Esse script roda quando o LocalStack está pronto e cria três filas FIFO do SQS. Primeiro cria a DLQ porque a fila principal precisa do ARN dela. Depois cria a fila principal configurando o redrive para essa DLQ. Por fim cria a fila de eventos de domínio."

# awslocal = AWS CLI apontando para o LocalStack (não é AWS real).
set -euo pipefail

echo "localstack: creating SQS FIFO queues..."

# --- 1) DLQ primeiro (a fila principal precisa do ARN dela no RedrivePolicy) ---
# A fila principal terá uma configuração dizendo:

# "Se uma mensagem falhar demais, mande para esta DLQ."

# Para fazer isso, ela precisa conhecer o ARN da DLQ.
DLQ_URL=$(awslocal sqs create-queue \
  --queue-name wager-transactions-dlq.fifo \
  --attributes FifoQueue=true,ContentBasedDeduplication=false \
  --query 'QueueUrl' --output text)

# ARN é o identificador usado na política de redrive (não a URL).
DLQ_ARN=$(awslocal sqs get-queue-attributes \
  --queue-url "$DLQ_URL" \
  --attribute-names QueueArn \
  --query 'Attributes.QueueArn' --output text)

# --- 2) Fila de entrada + redrive para a DLQ ---
# FifoQueue=true              → ordenação / grupo (MessageGroupId = walletId no app)
# ContentBasedDeduplication=false → dedup explícita via MessageDeduplicationId
# VisibilityTimeout=30        → tempo em que a msg fica "invisível" após Receive
# RedrivePolicy maxReceiveCount=5 → após 5 receives sem delete, vai para a DLQ
awslocal sqs create-queue \
  --queue-name wager-transactions.fifo \
  --attributes "{
    \"FifoQueue\": \"true\",
    \"ContentBasedDeduplication\": \"false\",
    \"VisibilityTimeout\": \"30\",
    \"RedrivePolicy\": \"{\\\"deadLetterTargetArn\\\":\\\"${DLQ_ARN}\\\",\\\"maxReceiveCount\\\":\\\"5\\\"}\"
  }" >/dev/null

# --- 3) Fila de eventos de domínio (outbox publisher publica aqui) ---
awslocal sqs create-queue \
  --queue-name domain-events.fifo \
  --attributes FifoQueue=true,ContentBasedDeduplication=false >/dev/null

echo "localstack: SQS ready"
awslocal sqs list-queues
