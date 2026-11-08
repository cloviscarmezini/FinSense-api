# Especificação Técnica do Backend FinSense & Two-Way Sync Engine (Agent-First)

> **Documento Oficial de Engenharia de Software Orientado a Agentes Autônomos (Agent-First)**  
> **Versão da Especificação:** 2.1.0  
> **Status:** Aprovado para Implementação e Consumo Automatizado  
> **Data de Emissão:** 2026-11-08  
> **Target:** Backend Developers, AI Coding Agents, Cloud Engineers & Client Integrators.

---

## 1. Manifesto do Sistema & Índice de Navegação para Agentes

```yaml
system_manifest:
  name: "FinSense Backend, Cloud AI & Two-Way Sync Service"
  version: "2.1.0"
  architecture: "Offline-First Hybrid Sync + Serverless Object Storage + Remote AI Server"
  runtime_target: "Go / Node.js (TypeScript) / Python (FastAPI) / Rust"
  containerization: "Docker 100% + Docker Compose para Execução Local Unificada"
  api_documentation: "OpenAPI 3.1 / Swagger UI interativo (/docs e /swagger)"
  primary_database: "PostgreSQL 16+ com uuid-ossp e citext"
  blob_storage: "S3-compatible (MinIO para ambiente local / AWS S3 ou Cloudflare R2 em produção)"
  sync_protocol: "Monotonic Revision-Based Log Sync (Push-Pull Delta com LWW)"
  ai_runtime: "llama.cpp server / vLLM / Ollama wrapper (GGUF Q4_K_M/Q8_0)"
  base_url_production: "https://api.finsense.com.br/v1"
  base_url_sandbox: "https://sandbox-api.finsense.com.br/v1"
  base_url_local: "http://localhost:8080/api/v1"
  swagger_ui_local: "http://localhost:8080/docs (ou http://localhost:8081 via container dedicado)"
```

### 1.1. Índice de Endpoints Rápidos (Quick API Matrix)

| Método | Caminho | Autenticação | Finalidade |
| :--- | :--- | :--- | :--- |
| `GET` | `/docs` / `/swagger` | Pública | **Swagger UI**: Interface interativa de documentação da API |
| `GET` | `/openapi.json` | Pública | Especificação canônica OpenAPI 3.1 em formato JSON |
| `POST` | `/auth/register` | Pública | Registro de nova conta de usuário |
| `POST` | `/auth/login` | Pública | Autenticação com credenciais (retorna JWT Access + Refresh) |
| `POST` | `/auth/refresh` | Pública | Renovação de Access Token via Refresh Token |
| `POST` | `/auth/logout` | Bearer Token | Revogação de sessão |
| `GET` | `/user/me` | Bearer Token | Leitura do perfil e limites do usuário |
| `POST` | `/sync/push-pull` | Bearer Token | **Core Sync**: Envio de mutações locais e recebimento de deltas |
| `GET` | `/sync/delta` | Bearer Token | Pull puro de mutações desde `since_revision` |
| `POST` | `/sync/reset` | Bearer Token | Exclusão seletiva de bases de dados na nuvem |
| `POST` | `/assets/presigned-upload` | Bearer Token | Geração de URL pré-assinada para upload de imagem/PDF no S3/MinIO |
| `POST` | `/assets/confirm-upload` | Bearer Token | Validação de integridade do upload no bucket |
| `GET` | `/assets/:id/download-url` | Bearer Token | Emissão de URL pré-assinada de leitura para download/exibição |
| `GET` | `/backup/export-full` | Bearer Token | Download de arquivo de backup binário LZFSE (`FCBK`) |
| `POST` | `/backup/import-selective`| Bearer Token | Importação e validação de backup granular |
| `GET` | `/ai/models` | Bearer Token | **IA**: Lista catálogo sugerido e modelos baixados no servidor |
| `POST` | `/ai/models/download` | Bearer Token | **IA**: Dispara download assíncrono de modelo do Hugging Face |
| `GET` | `/ai/models/download/:task_id`| Bearer Token | **IA**: Consulta status e progresso do download do modelo |
| `POST` | `/ai/models/download/:task_id/cancel`| Bearer Token | **IA**: Cancela download de modelo em andamento |
| `POST` | `/ai/models/select` | Bearer Token | **IA**: Define o modelo ativo para inferência |
| `DELETE`| `/ai/models/:model_id` | Bearer Token | **IA**: Exclui modelo do servidor para liberar espaço em disco |
| `POST` | `/ai/chat/completions` | Bearer Token | **IA**: Chat financeiro (suporte a SSE streaming e JSON) |
| `POST` | `/ai/analyze-finances` | Bearer Token | **IA**: Diagnóstico estruturado de fluxo, dívidas e contratos |
| `POST` | `/ai/parse-statement` | Bearer Token | **IA**: Extração estruturada de transações de extrato/OCR |

---

## 2. Configuração 100% Docker & Docker Compose (Execução Local Unificada)

O backend deve ser entregue com configuração completa para subir todo o ambiente de desenvolvimento e testes locais com **um único comando**, sem necessidade de instalar PostgreSQL, MinIO ou bibliotecas de IA manualmente no sistema operacional host.

```bash
# Inicialização completa de todo o ecossistema local:
docker compose up -d
```

### 2.1. Arquitetura dos Serviços no Docker Compose

```
+----------------------------------------------------------------------------------------+
|                                REDE DOCKER: finsense-network                           |
|                                                                                        |
|  [finsense-api] (Porta 8080)                                                           |
|    ├── Conecta em finsense-db:5432 (PostgreSQL 16)                                     |
|    ├── Conecta em finsense-storage:9000 (MinIO S3)                                     |
|    ├── Conecta em finsense-ai:8000 (llama.cpp server)                                  |
|    └── Serve Swagger UI em http://localhost:8080/docs                                  |
|                                                                                        |
|  [finsense-db] (Porta 5432)                                                            |
|    └── PostgreSQL 16 com extensões 'uuid-ossp' e 'citext' pré-carregadas                |
|                                                                                        |
|  [finsense-storage] (Portas 9000 S3 / 9001 Console Web)                                |
|    └── MinIO S3 compatível com auto-criação do bucket 'finsense-assets'                |
|                                                                                        |
|  [finsense-ai] (Porta 8001 / interna 8000)                                             |
|    └── Servidor de inferência GGUF montando o volume compartilhado ./models            |
|                                                                                        |
|  [finsense-swagger-ui] (Porta 8081)                                                    |
|    └── Interface Swagger UI autônoma espelhando /api/v1/openapi.json                  |
+----------------------------------------------------------------------------------------+
```

---

### 2.2. Arquivo `docker-compose.yml` de Produção/Local

O arquivo `docker-compose.yml` abaixo deve ser posicionado na raiz do projeto backend (`FinSense-api/docker-compose.yml`):

```yaml
version: "3.9"

services:
  # 1. API Principal FinSense
  finsense-api:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: finsense-api
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - PORT=8080
      - ENVIRONMENT=local
      - DATABASE_URL=postgres://finsense:finsense_secret@finsense-db:5432/finsense_db?sslmode=disable
      - JWT_SECRET=finsense_super_secret_jwt_key_local_development_2026_xyz
      - JWT_ACCESS_EXPIRATION_MINUTES=15
      - JWT_REFRESH_EXPIRATION_DAYS=30
      - S3_ENDPOINT=http://finsense-storage:9000
      - S3_PUBLIC_URL=http://localhost:9000
      - S3_BUCKET=finsense-assets
      - S3_ACCESS_KEY=minioadmin
      - S3_SECRET_KEY=minioadmin
      - S3_REGION=us-east-1
      - AI_ENGINE_URL=http://finsense-ai:8000
      - MODELS_DIR=/app/models
      - SWAGGER_ENABLED=true
    volumes:
      - ./models:/app/models
    depends_on:
      finsense-db:
        condition: service_healthy
      finsense-storage:
        condition: service_healthy
    networks:
      - finsense-network

  # 2. Banco de Dados PostgreSQL 16
  finsense-db:
    image: postgres:16-alpine
    container_name: finsense-db
    restart: unless-stopped
    ports:
      - "5432:5432"
    environment:
      - POSTGRES_USER=finsense
      - POSTGRES_PASSWORD=finsense_secret
      - POSTGRES_DB=finsense_db
    volumes:
      - pgdata:/var/lib/postgresql/data
      - ./scripts/init-db.sql:/docker-entrypoint-initdb.d/init.sql:ro
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U finsense -d finsense_db"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - finsense-network

  # 3. Object Storage S3-Compatível (MinIO)
  finsense-storage:
    image: minio/minio:RELEASE.2024-05-10T01-41-38Z
    container_name: finsense-storage
    restart: unless-stopped
    ports:
      - "9000:9000"
      - "9001:9001"
    environment:
      - MINIO_ROOT_USER=minioadmin
      - MINIO_ROOT_PASSWORD=minioadmin
    command: server /data --console-address ":9001"
    volumes:
      - miniodata:/data
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:9000/minio/health/live"]
      interval: 5s
      timeout: 5s
      retries: 5
    networks:
      - finsense-network

  # 4. Inicializador do Bucket MinIO
  finsense-storage-init:
    image: minio/mc:latest
    container_name: finsense-storage-init
    depends_on:
      finsense-storage:
        condition: service_healthy
    entrypoint: >
      /bin/sh -c "
      /usr/bin/mc alias set local http://finsense-storage:9000 minioadmin minioadmin;
      /usr/bin/mc mb --ignore-existing local/finsense-assets;
      /usr/bin/mc anonymous set download local/finsense-assets;
      exit 0;
      "
    networks:
      - finsense-network

  # 5. Servidor de Inferência de IA (llama.cpp server para GGUF)
  finsense-ai:
    image: ghcr.io/ggerganov/llama.cpp:server
    container_name: finsense-ai
    restart: unless-stopped
    ports:
      - "8001:8000"
    volumes:
      - ./models:/models
    command: >
      --host 0.0.0.0 --port 8000
      --ctx-size 4096 --n-gpu-layers 0
    networks:
      - finsense-network

  # 6. Swagger UI Autônomo
  finsense-swagger-ui:
    image: swaggerapi/swagger-ui:latest
    container_name: finsense-swagger-ui
    restart: unless-stopped
    ports:
      - "8081:8080"
    environment:
      - URL=http://localhost:8080/api/v1/openapi.json
    depends_on:
      - finsense-api
    networks:
      - finsense-network

volumes:
  pgdata:
    driver: local
  miniodata:
    driver: local

networks:
  finsense-network:
    driver: bridge
```

---

### 2.3. Multi-Stage Dockerfile de Produção da API

```dockerfile
# Estágio 1: Build
FROM golang:1.22-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s" \
    -o /app/bin/finsense-api ./cmd/api

# Estágio 2: Imagem Final Mínima e Segura
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl && \
    addgroup -S finsense && adduser -S finsense -G finsense

COPY --from=builder /app/bin/finsense-api /app/finsense-api
COPY --from=builder /app/docs /app/docs

RUN mkdir -p /app/models && chown -R finsense:finsense /app

USER finsense

EXPOSE 8080

HEALTHCHECK --interval=10s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/finsense-api"]
```

---

### 2.4. Arquivo `.env.example` Documentado

```bash
# Servidor
PORT=8080
ENVIRONMENT=local # local | staging | production
SWAGGER_ENABLED=true

# Banco de Dados PostgreSQL
DATABASE_URL=postgres://finsense:finsense_secret@localhost:5432/finsense_db?sslmode=disable
DB_MAX_OPEN_CONNS=25
DB_MAX_IDLE_CONNS=10
DB_CONN_MAX_LIFETIME=5m

# Autenticação JWT
JWT_SECRET=finsense_super_secret_jwt_key_local_development_2026_xyz
JWT_ACCESS_EXPIRATION_MINUTES=15
JWT_REFRESH_EXPIRATION_DAYS=30

# Armazenamento de Arquivos S3 / MinIO
S3_ENDPOINT=http://localhost:9000
S3_PUBLIC_URL=http://localhost:9000
S3_BUCKET=finsense-assets
S3_ACCESS_KEY=minioadmin
S3_SECRET_KEY=minioadmin
S3_REGION=us-east-1
S3_USE_SSL=false

# Motor de IA
AI_ENGINE_URL=http://localhost:8001
MODELS_DIR=./models
HUGGING_FACE_DEFAULT_TOKEN=
```

---

## 3. Documentação Interativa da API (OpenAPI 3.1 & Swagger)

O projeto backend **deve obrigatoriamente** expor a especificação da API de forma interativa e visual, permitindo que desenvolvedores e agentes inspecionem e executem chamadas de teste diretamente pelo navegador.

### 3.1. URLs de Acesso à Documentação
- **Swagger UI Embutido na API**: `http://localhost:8080/docs` (ou `/swagger`)
- **Documentação ReDoc (Visual Moderno)**: `http://localhost:8080/redoc`
- **JSON OpenAPI 3.1 Canônico**: `http://localhost:8080/api/v1/openapi.json`
- **YAML OpenAPI 3.1 Canônico**: `http://localhost:8080/api/v1/openapi.yaml`
- **Swagger UI Container Standalone**: `http://localhost:8081`

### 3.2. Estrutura do Esquema OpenAPI 3.1 (Amostra Canônica)

```json
{
  "openapi": "3.1.0",
  "info": {
    "title": "FinSense Core & AI Backend API",
    "version": "2.1.0",
    "description": "API RESTful de sincronização bidirecional offline-first e orquestração de IA para o ecossistema FinSense."
  },
  "servers": [
    {
      "url": "http://localhost:8080/api/v1",
      "description": "Servidor Local (Docker Compose)"
    },
    {
      "url": "https://api.finsense.com.br/v1",
      "description": "Servidor de Produção"
    }
  ],
  "components": {
    "securitySchemes": {
      "BearerAuth": {
        "type": "http",
        "scheme": "bearer",
        "bearerFormat": "JWT",
        "description": "Insira o token JWT retornado pelo endpoint /auth/login."
      }
    }
  },
  "security": [
    {
      "BearerAuth": []
    }
  ],
  "tags": [
    { "name": "Autenticação", "description": "Endpoints de login, registro e renovação de tokens" },
    { "name": "Sincronização", "description": "Core Two-Way Sync (Push-Pull Delta com LWW)" },
    { "name": "Inteligência Artificial", "description": "Gerenciamento de modelos GGUF, download do Hugging Face e inferência" },
    { "name": "Assets", "description": "Upload e download de comprovantes via Presigned URLs" },
    { "name": "Backup", "description": "Exportação consolidada comprimida (FCBK) e importação" }
  ]
}
```

---

## 4. Esquema Relacional de Dados (PostgreSQL DDL)

O script abaixo é executado automaticamente pelo container `finsense-db` no boot inicial através do volume `./scripts/init-db.sql`:

```sql
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";

-- 1. Usuários
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email CITEXT UNIQUE NOT NULL,
    password_hash VARCHAR(255) NOT NULL,
    full_name VARCHAR(150) NOT NULL,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 2. Changelog Global de Sincronização
CREATE TABLE sync_changelog (
    revision BIGSERIAL PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    client_id VARCHAR(100) NOT NULL,
    entity_name VARCHAR(50) NOT NULL,
    entity_id UUID NOT NULL,
    operation VARCHAR(10) NOT NULL CHECK (operation IN ('INSERT', 'UPDATE', 'DELETE')),
    mutation_id UUID UNIQUE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_sync_changelog_user_rev ON sync_changelog(user_id, revision);

-- 3. Assets Binários
CREATE TABLE asset_files (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    file_name VARCHAR(255) NOT NULL,
    mime_type VARCHAR(100) NOT NULL,
    size_bytes BIGINT NOT NULL,
    sha256_hash VARCHAR(64) NOT NULL,
    storage_key VARCHAR(500) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_asset_files_user ON asset_files(user_id) WHERE deleted_at IS NULL;

-- 4. Contas Bancárias
CREATE TABLE bank_accounts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    bank_preset_id VARCHAR(50),
    custom_image_path VARCHAR(500),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'building.columns.fill',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#EC7000',
    initial_balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    agency VARCHAR(20) NOT NULL DEFAULT '',
    account_number VARCHAR(30) NOT NULL DEFAULT '',
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_bank_accounts_user ON bank_accounts(user_id) WHERE deleted_at IS NULL;

-- 5. Categorias
CREATE TABLE categories (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(100) NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('expense', 'income')),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'tag.fill',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#6366F1',
    keywords TEXT[] NOT NULL DEFAULT '{}',
    is_system_default BOOLEAN NOT NULL DEFAULT FALSE,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_categories_user_type ON categories(user_id, type) WHERE deleted_at IS NULL;

-- 6. Transações Financeiras
CREATE TABLE financial_transactions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    name VARCHAR(200) NOT NULL,
    details TEXT NOT NULL DEFAULT '',
    amount NUMERIC(15, 2) NOT NULL CHECK (amount >= 0),
    date TIMESTAMPTZ NOT NULL,
    type VARCHAR(10) NOT NULL CHECK (type IN ('expense', 'income')),
    is_paid BOOLEAN NOT NULL DEFAULT TRUE,
    attachment_path VARCHAR(500),
    attachment_name VARCHAR(255),
    ignore_in_consolidated BOOLEAN NOT NULL DEFAULT FALSE,
    recurrence_mode VARCHAR(20) NOT NULL DEFAULT 'single' 
        CHECK (recurrence_mode IN ('single', 'installment', 'repeated', 'recurring')),
    recurrence_frequency VARCHAR(20) 
        CHECK (recurrence_frequency IN ('weekly', 'biweekly', 'monthly', 'bimonthly', 'quarterly', 'semiannual', 'annual')),
    is_canceled_recurring_series BOOLEAN,
    series_group_id UUID,
    installment_index INT,
    installment_total INT,
    raw_statement_line TEXT,
    statement_occurrence_index INT NOT NULL DEFAULT 1,
    linked_contract_id UUID,
    linked_investment_id UUID,
    linked_debt_id UUID,
    linked_quota_id UUID,
    linked_quota_units INT,
    linked_quota_year_month_key INT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
CREATE INDEX idx_transactions_user_date ON financial_transactions(user_id, date) WHERE deleted_at IS NULL;
CREATE INDEX idx_transactions_series ON financial_transactions(series_group_id) WHERE deleted_at IS NULL;

-- 7. Investimentos e Movimentações
CREATE TABLE investment_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    investment_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    name VARCHAR(150) NOT NULL,
    group_name VARCHAR(150) NOT NULL,
    balance NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    show_in_consolidated BOOLEAN NOT NULL DEFAULT TRUE,
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'chart.line.uptrend.xyaxis',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#10B981',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE investment_movements (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    investment_item_id UUID NOT NULL REFERENCES investment_items(id) ON DELETE CASCADE,
    source_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    investment_account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    type VARCHAR(15) NOT NULL CHECK (type IN ('deposit', 'withdrawal')),
    amount NUMERIC(15, 2) NOT NULL CHECK (amount >= 0),
    date TIMESTAMPTZ NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    generated_transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 8. Contratos de Crédito e Amortizações
CREATE TABLE credit_contracts (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE RESTRICT,
    kind VARCHAR(15) NOT NULL CHECK (kind IN ('loan', 'financing')),
    name VARCHAR(150) NOT NULL,
    contract_number VARCHAR(100),
    loan_subtype VARCHAR(30) 
        CHECK (loan_subtype IN ('Consignado', 'Pessoal', 'Antecipação FGTS', 'Empresarial', 'Refinanciamento', 'Outros')),
    financing_subtype VARCHAR(20) 
        CHECK (financing_subtype IN ('Imóvel', 'Carro')),
    amortization_system VARCHAR(100) NOT NULL DEFAULT 'PRICE (Parcelas Fixas)',
    total_contracted_amount NUMERIC(15, 2) NOT NULL,
    outstanding_balance NUMERIC(15, 2),
    installment_amount NUMERIC(15, 2) NOT NULL,
    billing_day INT NOT NULL CHECK (billing_day BETWEEN 1 AND 31),
    total_installments INT NOT NULL CHECK (total_installments >= 1),
    monthly_interest_rate NUMERIC(8, 4) NOT NULL DEFAULT 0.0000,
    start_date TIMESTAMPTZ,
    past_installments_elapsed_at_creation INT NOT NULL DEFAULT 0,
    extra_amortized_total NUMERIC(15, 2) NOT NULL DEFAULT 0.00,
    is_settled BOOLEAN NOT NULL DEFAULT FALSE,
    settled_amount NUMERIC(15, 2),
    settled_date TIMESTAMPTZ,
    settled_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE amortization_records (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    contract_id UUID NOT NULL REFERENCES credit_contracts(id) ON DELETE CASCADE,
    account_id UUID NOT NULL REFERENCES bank_accounts(id) ON DELETE RESTRICT,
    transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    date TIMESTAMPTZ NOT NULL,
    amount_paid NUMERIC(15, 2) NOT NULL,
    mode VARCHAR(100) NOT NULL,
    previous_installment_amount NUMERIC(15, 2) NOT NULL,
    new_installment_amount NUMERIC(15, 2) NOT NULL,
    previous_remaining_installments INT NOT NULL,
    new_remaining_installments INT NOT NULL,
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 9. Dívidas
CREATE TABLE debt_items (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    custom_image_path VARCHAR(500),
    bank_preset_id VARCHAR(50),
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'creditcard.trianglebadge.exclamationmark',
    color_hex VARCHAR(9) NOT NULL DEFAULT '#EF4444',
    original_amount NUMERIC(15, 2) NOT NULL,
    contracted_date TIMESTAMPTZ,
    notes TEXT NOT NULL DEFAULT '',
    is_paid BOOLEAN NOT NULL DEFAULT FALSE,
    negotiated_amount NUMERIC(15, 2),
    paid_date TIMESTAMPTZ,
    paid_from_account_id UUID REFERENCES bank_accounts(id) ON DELETE SET NULL,
    payment_transaction_id UUID REFERENCES financial_transactions(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 10. Provisões por Categoria e Cotas de Itens Específicos
CREATE TABLE category_provisions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    year INT NOT NULL,
    month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    estimated_amount NUMERIC(15, 2) NOT NULL CHECK (estimated_amount >= 0),
    repeat_for_future_months BOOLEAN NOT NULL DEFAULT TRUE,
    monthly_overrides JSONB,
    excluded_year_month_keys INT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

CREATE TABLE category_quota_rules (
    id UUID PRIMARY KEY,
    series_id UUID NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id UUID NOT NULL REFERENCES categories(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    icon_symbol VARCHAR(100) NOT NULL DEFAULT 'shippingbox.fill',
    year INT NOT NULL,
    month INT NOT NULL CHECK (month BETWEEN 1 AND 12),
    planned_quantity INT NOT NULL CHECK (planned_quantity >= 0),
    estimated_unit_price NUMERIC(15, 2) NOT NULL CHECK (estimated_unit_price >= 0),
    repeat_for_future_months BOOLEAN NOT NULL DEFAULT TRUE,
    monthly_overrides JSONB,
    excluded_year_month_keys INT[],
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);

-- 11. Modelos de IA e Tarefas de Download no Backend
CREATE TABLE ai_models (
    id VARCHAR(150) PRIMARY KEY,
    display_name VARCHAR(150) NOT NULL,
    repo_id VARCHAR(150) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    local_path VARCHAR(500) NOT NULL,
    size_bytes BIGINT NOT NULL,
    quantization VARCHAR(50) NOT NULL DEFAULT 'Q4_K_M',
    is_active_for_user UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE ai_download_tasks (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    repo_id VARCHAR(150) NOT NULL,
    file_name VARCHAR(255) NOT NULL,
    status VARCHAR(30) NOT NULL CHECK (status IN ('PENDING', 'DOWNLOADING', 'VERIFYING', 'READY', 'FAILED', 'CANCELED')),
    progress_percentage NUMERIC(5, 2) NOT NULL DEFAULT 0.00,
    downloaded_bytes BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    speed_bytes_sec BIGINT NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX idx_ai_download_user ON ai_download_tasks(user_id, status);

CREATE TABLE chat_messages (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant', 'system')),
    content TEXT NOT NULL,
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    model_used VARCHAR(100),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMPTZ
);
```

---

## 5. Especificação Estrita de Endpoints de Inteligência Artificial (AI Engine)

O backend FinSense atua como o servidor de inferência e orquestrador de IA, permitindo que clientes móveis consumam inteligência avançada sem rodar LLMs localmente.

```
+-------------------------------------------------------------------------------+
|                       ARQUITETURA DO BACKEND AI ENGINE                        |
|                                                                               |
|  [Hugging Face Hub API] <--- Download Task ---> [Local Model Storage (.gguf)] |
|                                                                               |
|  [Client App] ---> POST /ai/chat/completions ---> [Llama.cpp / vLLM Server]   |
|                                                          ^                    |
|  [Context Builder] (Contas + Extratos + Contratos) ------|                    |
+-------------------------------------------------------------------------------+
```

---

### 5.1. `GET /api/v1/ai/models`
Retorna a lista de modelos suportados/sugeridos pelo FinSense e o estado atual de instalação no servidor para o usuário autenticado.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`
- **Response 200 OK**:
```json
{
  "selected_model_id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
  "installed_models": [
    {
      "id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
      "display_name": "Qwen 2.5 3B Instruct (GGUF)",
      "repo_id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
      "file_name": "qwen2.5-3b-instruct-q4_k_m.gguf",
      "size_bytes": 1950000000,
      "quantization": "Q4_K_M",
      "is_active": true,
      "downloaded_at": "2026-11-08T02:00:00Z"
    }
  ],
  "suggested_models": [
    {
      "id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
      "name": "Qwen 2.5 3B Instruct (GGUF)",
      "description": "Excelente em português, categorização bancária e cálculos financeiros.",
      "size_estimate": "~1.9 GB",
      "is_downloaded": true
    },
    {
      "id": "Qwen/Qwen2.5-1.5B-Instruct-GGUF",
      "name": "Qwen 2.5 1.5B Instruct (Ultra Leve)",
      "description": "Modelo compacto e de baixíssima latência.",
      "size_estimate": "~1.1 GB",
      "is_downloaded": false
    },
    {
      "id": "bartowski/Llama-3.2-3B-Instruct-GGUF",
      "name": "Llama 3.2 3B Instruct (GGUF)",
      "description": "Modelo rápido da Meta para análise de textos e relatórios.",
      "size_estimate": "~2.0 GB",
      "is_downloaded": false
    }
  ]
}
```

---

### 5.2. `POST /api/v1/ai/models/download`
Inicia um processo assíncrono em segundo plano no servidor para baixar o arquivo GGUF diretamente dos repositórios do Hugging Face.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`, `Content-Type: application/json`
- **Request Body**:
```json
{
  "repo_id": "Qwen/Qwen2.5-1.5B-Instruct-GGUF",
  "hugging_face_token": "hf_xxxxxxxxxxxxxxxxxxxxxx",
  "filename_filter": "q4_k_m"
}
```
- **Response 202 Accepted**:
```json
{
  "task_id": "9b1d6f2c-6d32-47a1-8894-67d0c5311f5e",
  "status": "PENDING",
  "message": "Download do modelo iniciado no servidor."
}
```

---

### 5.3. `GET /api/v1/ai/models/download/:task_id`
Consulta o progresso e a taxa de transferência em tempo real do download do modelo no servidor.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`
- **Response 200 OK**:
```json
{
  "task_id": "9b1d6f2c-6d32-47a1-8894-67d0c5311f5e",
  "repo_id": "Qwen/Qwen2.5-1.5B-Instruct-GGUF",
  "file_name": "qwen2.5-1.5b-instruct-q4_k_m.gguf",
  "status": "DOWNLOADING",
  "progress_percentage": 45.8,
  "downloaded_bytes": 503800000,
  "total_bytes": 1100000000,
  "speed_bytes_sec": 12500000,
  "error_message": null
}
```

---

### 5.4. `POST /api/v1/ai/models/download/:task_id/cancel`
Cancela a tarefa de download e purga arquivos temporários parciais no disco do servidor.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`
- **Response 200 OK**:
```json
{
  "task_id": "9b1d6f2c-6d32-47a1-8894-67d0c5311f5e",
  "status": "CANCELED",
  "message": "Download cancelado com sucesso."
}
```

---

### 5.5. `POST /api/v1/ai/models/select`
Define o modelo que responderá às chamadas de chat e diagnósticos do usuário.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`, `Content-Type: application/json`
- **Request Body**:
```json
{
  "model_id": "Qwen/Qwen2.5-3B-Instruct-GGUF"
}
```
- **Response 200 OK**:
```json
{
  "selected_model_id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
  "is_active": true
}
```

---

### 5.6. `DELETE /api/v1/ai/models/:model_id`
Remove fisicamente o modelo GGUF do disco do servidor.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`
- **Response 200 OK**:
```json
{
  "deleted_model_id": "Qwen/Qwen2.5-1.5B-Instruct-GGUF",
  "freed_bytes": 1100000000
}
```

---

### 5.7. `POST /api/v1/ai/chat/completions`
Executa a inferência de conversação financeira. Suporta resposta em bloco (`application/json`) e streaming Server-Sent Events (`text/event-stream`).

- **Headers**:
  - `Authorization: Bearer <JWT_TOKEN>`
  - `Content-Type: application/json`
  - `Accept: text/event-stream` (se streaming desejado) ou `application/json`
- **Request Body**:
```json
{
  "messages": [
    {
      "role": "system",
      "content": "Você é o assistente financeiro pessoal do FinSense. Contexto do usuário: Saldo em conta R$ 1.180,93; Dívida Cartão Sam's R$ 1.681,00; Receitas no mês R$ 3.139,38; Despesas no mês R$ 3.626,25."
    },
    {
      "role": "user",
      "content": "Em quanto tempo consigo quitar a dívida do Cartão Sam's?"
    }
  ],
  "temperature": 0.2,
  "max_tokens": 800,
  "stream": false
}
```
- **Response 200 OK (`application/json`)**:
```json
{
  "id": "chatcmpl-789a44b1",
  "model": "Qwen/Qwen2.5-3B-Instruct-GGUF",
  "created": 1794105000,
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "## Análise de Quitação: Cartão de crédito Sam’s\nAo analisar seu fluxo intra-mês, sua receita prevista para o dia 28/11 permitirá quitar o valor de R$ 1.681,00 mantendo saldo de segurança de R$ 350,00."
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 420,
    "completion_tokens": 110,
    "total_tokens": 530
  }
}
```

---

### 5.8. `POST /api/v1/ai/analyze-finances`
Endpoint analítico estruturado de alta precisão. O backend compila as contas, transações, contratos e dívidas do banco do usuário, processa no modelo de IA e retorna um diagnóstico com plano de ação cronológico.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`, `Content-Type: application/json`
- **Request Body**:
```json
{
  "target_month": "2026-11",
  "specific_debt_id": "50000000-0000-0000-0000-000000000001",
  "specific_contract_id": null
}
```
- **Response 200 OK**:
```json
{
  "analysis_date": "2026-11-08T00:00:00Z",
  "net_monthly_capacity": 486.87,
  "safe_payoff_date": "2026-11-28",
  "is_payoff_viable_this_month": true,
  "required_reserve_amount": 350.00,
  "action_plan_markdown": "### Plano de Ação\n- Aguardar o recebimento do Salário no dia 28/11.\n- Realizar a liquidação à vista com deságio sugerido de 15%.\n- Economia estimada em juros futuros: R$ 320,00.",
  "contracts_disambiguation": []
}
```

---

### 5.9. `POST /api/v1/ai/parse-statement`
Ingere texto bruto de extrato bancário (extraído de PDF ou OCR no cliente) e devolve as transações financeiras estruturadas e categorizadas em JSON estrito.

- **Headers**: `Authorization: Bearer <JWT_TOKEN>`, `Content-Type: application/json`
- **Request Body**:
```json
{
  "raw_text": "05/11 PIX ENVIADO MERCADO PAGO -150,00\n06/11 SALARIO EMPRESA +4500,00",
  "default_account_id": "30000000-0000-0000-0000-000000000001",
  "filter_mode": "hybrid"
}
```
- **Response 200 OK**:
```json
{
  "parsed_transactions": [
    {
      "date": "2026-11-05T00:00:00Z",
      "raw_description": "PIX ENVIADO MERCADO PAGO",
      "clean_title": "Mercado Pago",
      "details": "Transferência Pix enviada",
      "amount": 150.00,
      "type": "expense",
      "suggested_category_name": "Outros",
      "recurrence_mode": "single",
      "installments_count": 1,
      "occurrence_index": 1
    },
    {
      "date": "2026-11-06T00:00:00Z",
      "raw_description": "SALARIO EMPRESA",
      "clean_title": "Salário",
      "details": "Crédito em conta corrente",
      "amount": 4500.00,
      "type": "income",
      "suggested_category_name": "Salário",
      "recurrence_mode": "recurring",
      "installments_count": 1,
      "occurrence_index": 1
    }
  ]
}
```

---

## 6. Estrutura Canônica de Resposta de Erros (RFC 7807 Pattern)

Todo retorno com código HTTP diferente de 2xx segue rigidamente o formato:

```json
{
  "error": {
    "code": "MODEL_DOWNLOAD_FAILED",
    "message": "Falha de conexão com o repositório do Hugging Face. Verifique seu token de acesso.",
    "status_code": 401,
    "timestamp": "2026-11-08T03:22:10.123Z",
    "details": {
      "repo_id": "Qwen/Qwen2.5-3B-Instruct-GGUF",
      "upstream_status": 401
    }
  }
}
```

### Códigos de Erro Padronizados
- `AUTH_INVALID_CREDENTIALS`: Email ou senha incorretos.
- `AUTH_TOKEN_EXPIRED`: JWT expirado (necessário refresh).
- `SYNC_INVALID_PAYLOAD`: Payload de mutação malformado ou UUID inválido.
- `AI_MODEL_NOT_FOUND`: Modelo solicitado não está instalado no servidor.
- `AI_MODEL_BUSY`: Servidor de inferência processando limite máximo de concorrência.
- `AI_DOWNLOAD_IN_PROGRESS`: Tarefa de download já em execução para este modelo.
- `AI_INSUFFICIENT_STORAGE`: Disco do servidor sem espaço livre para baixar o modelo GGUF.

---
*Fim da Especificação Técnica Oficial FinSense Backend (Agent-First).*
