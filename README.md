# FinSense Backend & Two-Way Sync Service (v2.1.0)

> **Serviço de Backend RESTful, Two-Way Sync Engine Offline-First e Orquestrador de IA Local para o Ecossistema FinSense.**

---

## 1. Visão Geral da Arquitetura

O backend do FinSense foi projetado para operar com tolerância a falhas, suporte a clientes offline-first e inferência de modelos de IA locais em formato GGUF.

```
+----------------------------------------------------------------------------------------+
|                                REDE DOCKER: finsense-network                           |
|                                                                                        |
|  [finsense-ai-manager] (Porta 8082) <--- EXCLUSIVO ADMIN                                 |
|    ├── Frontend Web Dashboard (http://localhost:8082/)                                 |
|    ├── Gestão Global de Modelos (Download HF, Progresso, Cancelamento, Seleção, Delete)|
|    └── Informa o modelo ativo global e gerencia a inferência                           |
|                                                                                        |
|  [finsense-api] (Porta 8080) <--- USADO PELOS CLIENTES / APPS                           |
|    ├── Conecta em finsense-db:5432 (PostgreSQL 16)                                     |
|    ├── Conecta em finsense-storage:9000 (MinIO S3)                                     |
|    ├── Conecta em finsense-ai-manager:8082 (Consulta Modelo Ativo Global)              |
|    ├── Chat com Contexto Isolado do Usuário Logado (Saldo, Contas, Dívidas)            |
|    └── Serve Swagger UI em http://localhost:8080/docs                                  |
|                                                                                        |
|  [finsense-db] (Porta 5432)                                                            |
|    └── PostgreSQL 16 com extensões 'uuid-ossp' e 'citext'                              |
|                                                                                        |
|  [finsense-storage] (Portas 9000 S3 / 9001 Console Web)                                |
|    └── MinIO S3 compatível com auto-criação do bucket 'finsense-assets'                |
|                                                                                        |
|  [finsense-swagger-ui] (Porta 8081)                                                    |
|    └── Interface Swagger UI autônoma espelhando /api/v1/openapi.json                  |
+----------------------------------------------------------------------------------------+
```

---

## 2. Inicialização Rápida (100% Docker)

Com apenas um comando, todo o ambiente sobe automaticamente (banco, storage S3, microserviço de IA com dashboard, Swagger UI e API em Go):

```bash
docker compose up -d
```

### URLs de Acesso Local
- **API Principal (Apps Móveis)**: [http://localhost:8080/api/v1](http://localhost:8080/api/v1)
- **Painel Web de Gestão de Modelos de IA (Admin)**: [http://localhost:8082/](http://localhost:8082/)
- **Swagger UI (Embutido)**: [http://localhost:8080/docs](http://localhost:8080/docs)
- **Swagger UI (Container Standalone)**: [http://localhost:8081](http://localhost:8081)
- **OpenAPI 3.1 JSON**: [http://localhost:8080/api/v1/openapi.json](http://localhost:8080/api/v1/openapi.json)
- **MinIO Console**: [http://localhost:9001](http://localhost:9001) (Usuário: `minioadmin` / Senha: `minioadmin`)
- **MinIO Console**: [http://localhost:9001](http://localhost:9001) (Usuário: `minioadmin` / Senha: `minioadmin`)

---

## 3. Matriz de Endpoints

### Autenticação & Usuários
| Método | Caminho | Autenticação | Descrição |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/auth/register` | Pública | Registro de nova conta (gera par Access + Refresh Token) |
| `POST` | `/api/v1/auth/login` | Pública | Autenticação de usuário com email e senha |
| `POST` | `/api/v1/auth/refresh` | Pública | Renovação do Access Token via Refresh Token |
| `POST` | `/api/v1/auth/logout` | Bearer Token | Encerramento de sessão |
| `GET` | `/api/v1/user/me` | Bearer Token | Consulta do perfil e status da conta |

### Core Two-Way Sync (Offline-First)
| Método | Caminho | Autenticação | Descrição |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/sync/push-pull` | Bearer Token | Envio de mutações locais e recebimento imediato de deltas |
| `GET` | `/api/v1/sync/delta` | Bearer Token | Pull puro de mutações desde `since_revision` |
| `POST` | `/api/v1/sync/reset` | Bearer Token | Reset/purga de dados na nuvem |

### Comprovantes & Arquivos (Assets)
| Método | Caminho | Autenticação | Descrição |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/assets/presigned-upload` | Bearer Token | Emissão de URL pré-assinada para upload direto no bucket S3 |
| `POST` | `/api/v1/assets/confirm-upload` | Bearer Token | Validação de integridade e registro do asset |
| `GET` | `/api/v1/assets/:id/download-url` | Bearer Token | Emissão de URL pré-assinada de leitura para download/exibição |

### Backup Granular
| Método | Caminho | Autenticação | Descrição |
| :--- | :--- | :--- | :--- |
| `GET` | `/api/v1/backup/export-full` | Bearer Token | Exportação completa dos dados do usuário em JSON consolidado |
| `POST` | `/api/v1/backup/import-selective` | Bearer Token | Importação e validação de backup granular |

### Inteligência Artificial (AI Engine para Apps Móveis - 100% Contexto Isolado do Usuário)
| Método | Caminho | Autenticação | Descrição |
| :--- | :--- | :--- | :--- |
| `POST` | `/api/v1/ai/chat/completions` | Bearer Token | Chat financeiro inteligente com injeção automática e isolada do contexto financeiro do usuário logado (usando o modelo global ativo) |
| `POST` | `/api/v1/ai/analyze-finances` | Bearer Token | Diagnóstico analítico de fluxo de caixa e amortização de dívidas |
| `POST` | `/api/v1/ai/parse-statement` | Bearer Token | Extração estruturada de transações de extrato/OCR |

### Gestão Global de Modelos de IA (Microserviço Administrativo Dedicado em `:8082`)
> **Acesso Exclusivo para Administradores / Dashboard Web:** [http://localhost:8082/](http://localhost:8082/)

| Método | Caminho | Finalidade |
| :--- | :--- | :--- |
| `GET` | `/` ou `/admin` | **Dashboard Web**: Interface gráfica para download, status, seleção de modelo ativo e exclusão |
| `GET` | `/api/admin/models` | Lista catálogo sugerido e modelos GGUF baixados no servidor |
| `POST` | `/api/admin/models/download` | Disparo assíncrono de download do Hugging Face para o storage global |
| `GET` | `/api/admin/models/download/:task_id` | Consulta de progresso e velocidade de download em tempo real |
| `POST` | `/api/admin/models/download/:task_id/cancel` | Cancelamento de download em andamento |
| `POST` | `/api/admin/models/select` | **Define o modelo ativo global do sistema** (utilizado por todas as chamadas de chat) |
| `DELETE` | `/api/admin/models/:model_id` | Remoção de modelo do disco |

---

## 4. Estrutura do Projeto

```
FinSense-api/
├── .env.example              # Exemplo de configuração de ambiente
├── .gitignore                # Regras de exclusão do Git
├── Dockerfile                # Multi-stage Dockerfile Go 1.22
├── docker-compose.yml        # Orquestração local unificada (6 serviços)
├── go.mod                    # Módulo Go e dependências
├── README.md                 # Documentação do projeto
├── cmd/
│   └── api/
│       └── main.go           # Ponto de entrada do servidor Go
├── docs/
│   ├── openapi.json          # Especificação OpenAPI 3.1 canônica
│   └── swagger.html          # Interface Swagger UI interativa
├── internal/
│   ├── api/
│   │   ├── handlers/         # Handlers HTTP REST
│   │   ├── middleware/       # Middlewares (Auth JWT, CORS, Logger, Recover)
│   │   └── router.go         # Definição e agrupamento de rotas com Chi
│   ├── config/               # Carregador de variáveis de ambiente
│   ├── database/             # Pool de conexões PostgreSQL (pgxpool)
│   ├── models/               # Entidades de domínio, DTOs e sync
│   ├── services/             # Regras de negócio (Auth, Storage S3, Sync, AI)
│   └── utils/                # Utilitários (JWT, RFC 7807 Errors)
├── models/                   # Diretório de modelos GGUF locais
└── scripts/
    └── init-db.sql           # Script DDL com tabelas e índices PostgreSQL
```

---

## 5. Respostas de Erro Padronizadas (RFC 7807)

Todas as respostas de erro HTTP seguem o padrão canônico:

```json
{
  "error": {
    "code": "AUTH_INVALID_CREDENTIALS",
    "message": "Email ou senha incorretos.",
    "status_code": 401,
    "timestamp": "2026-11-08T03:22:10.123Z",
    "details": null
  }
}
```
