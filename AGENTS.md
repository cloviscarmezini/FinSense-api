# Manifesto do Agente Antigravity — FinSense API & Sync Engine

> **Documento Canônico de Arquitetura, Regras de Negócio e Diretrizes de Engenharia para Agentes Autônomos**  
> **Escopo:** Backend, API REST, Cloud Sync e Microserviços (`FinSense-api`)  
> **Linguagem / Stack:** Go 1.22+ / Gin / GORM / PostgreSQL 16+ / Docker & Docker Compose / OpenAPI 3.1  
> **Repositório:** `FinSense-api` (Backend & Sync)

---

## ⚠️ Instruções Obrigatórias e Inegociáveis para Qualquer Agente

1. **Revisão e Manutenção Contínua deste Manifesto:**  
   Sempre que implementar novas rotas, adicionar tabelas no banco de dados, alterar esquemas de mutação do motor de sync ou modificar regras de negócio, você **DEVE** revisar e atualizar este manifesto (`AGENTS.md`), mantendo-o rigorosamente alinhado à implementação.
2. **Commits Semânticos Obrigatórios & Proibição de `git push`:**  
   Toda alteração de código ou configuração deve ser acompanhada de commit semântico no padrão *Conventional Commits* (ex.: `feat:`, `fix:`, `refactor:`, `test:`, `docs:`, `perf:`). **NUNCA execute `git push`** sob hipótese alguma para branches remotas; os commits devem permanecer estritamente no repositório local.
3. **Validação de Compilação e Testes em Go:**  
   Antes de finalizar tarefas, execute `go vet ./...` e `go test ./...` para assegurar que não haja quebras de tipos, dependências quebradas ou falhas de regressão no motor de sincronização.
4. **Isolamento e Compatibilidade Multi-Dispositivo (Desktop + iOS):**  
   O backend atende múltiplos clientes simultâneos (Desktop macOS e App iOS). Toda alteração nos esquemas de sincronização deve garantir compatibilidade retroativa e ordenação topológica de chaves estrangeiras.
5. **Paridade com Novos Campos e Entidades dos Apps:**  
   Sempre que novas features, entidades ou campos forem criados no Desktop ou iOS, a API **DEVE** obrigatoriamente atualizar seus modelos GORM, tabelas do banco, DTOs de sincronização e handlers de backup/sync.

---

## 1. Visão Geral da Arquitetura do Backend

```
FinSense-api (Go Backend)
├── cmd/
│   ├── server/main.go               # Ponto de entrada da API REST e Sync Engine
│   └── ai-manager/main.go           # Microserviço de gestão e download de modelos LLM
├── internal/
│   ├── api/
│   │   ├── handlers/                # Controllers HTTP (auth, sync, backup, assets, ai, health)
│   │   ├── middleware/              # JWT Auth, CORS, Logger, Panic Recovery
│   │   └── router.go                # Mapeamento central de rotas e grupos de endpoints
│   ├── config/                      # Configurações de ambiente (.env, portas, segredos)
│   ├── database/                    # Conexão PostgreSQL (GORM / pgx) e AutoMigrate
│   ├── models/                      # Entidades GORM e DTOs de sincronização (domain.go, sync.go)
│   ├── services/                    # Lógica de negócio (SyncService, AuthService, StorageService)
│   └── utils/                       # Utilitários de JWT, erros padronizados e hashing bcrypt
├── docs/                            # Swagger UI e especificação OpenAPI 3.1
├── Dockerfile                       # Build multistage para produção e testes
└── docker-compose.yml               # Orquestração local: Postgres 16, MinIO, API e Swagger UI
```

---

## 2. Catálogo de Entidades e Regras de Negócio do Ecossistema

### 2.1. Entidades Sincronizadas (Two-Way Delta Sync)
Todas as entidades do ecossistema financeiro possuem persistência relacional no PostgreSQL e são sincronizadas por deltas:

1. **`bank_accounts` (`BankAccount`):** Contas correntes, poupanças e carteiras do usuário.
2. **`categories` (`Category`):** Categorias de receita e despesa, palavras-chave para classificação automática.
3. **`financial_transactions` (`FinancialTransaction`):** Lançamentos financeiros com suporte a conciliação (`is_paid`), parcelamento (`recurrence_mode`), faturas de cartão (`credit_card_id`, `credit_card_invoice_id`) e amortizações de empréstimo.
4. **`credit_cards` (`CreditCard`):** Cartões com `closing_day`, `best_purchase_day`, `due_day`, `total_limit` e vínculo com conta padrão.
5. **`credit_card_invoices` (`CreditCardInvoice`):** Faturas mensais geradas com ciclo de datas, status de pagamento e vínculo à transação de liquidação.
6. **`investment_items` & `investment_movements`:** Portfólio de investimentos com opção de inclusão no saldo consolidado (`show_in_consolidated`).
7. **`credit_contracts` (`CreditContract`):** Contratos de financiamento habitacional, veicular ou empréstimo consignado/pessoal (sistemas SAC e PRICE).
8. **`amortization_plans` & `amortization_records`:** Camada 2 (planejamento extraordinário) e Camada 3 (quitação efetiva de contratos).
9. **`debt_items` (`DebtItem`):** Dívidas em aberto (ex.: renegociações Serasa) com valor original e valor de negociação.
10. **`category_provisions` (`CategoryProvision`):** Tetos orçamentários mensais e anuais por categoria com controle de escopo.
11. **`category_quota_rules` (`CategoryQuotaRule`):** Cotas de itens específicos (quantidade x preço unitário).
12. **`sinking_funds` (`SinkingFund`):** Caixinhas de provisão proporcional para despesas periódicas (IPVA, seguros, IPTU).
13. **`what_if_scenarios` (`WhatIfScenario`):** Cenários de teste de estresse de caixa e comparadores de amortização.
14. **`financial_insights` (`FinancialInsight`):** Alertas de ML comportamental com severidade (`critical`, `warning`, `info`) e feedback do usuário.
15. **`monthly_reviews` (`MonthlyReview`):** Fechamento mensal guiado comparando planejado vs. realizado.
16. **`ml_behavioral_profiles` (`MLBehavioralProfile`):** Pesos e parâmetros calibrados localmente pelo motor de machine learning.

### 2.2. Motor de Sincronização Two-Way (Delta Engine)
- **Protocolo Baseado em Revisões Monótonas:**
  - Cada mutação aplicada no servidor incrementa `users.current_revision`.
  - A tabela `sync_changelogs` registra cada operação (`INSERT`, `UPDATE`, `DELETE`, `UPSERT`) com timestamp e ID de mutação idempotente (`mutation_id`).
- **Resolução de Conflitos Last-Write-Wins (LWW):**
  - Compara o campo `updated_at` recebido com o registro existente.
  - Trata o caso de *ressurreição segura*: se uma entidade foi marcada com `deleted_at` anteriormente mas o cliente envia um `insert`/`upsert` posterior com `updated_at` mais recente, o registro é restaurado.
- **Ordenação Topológica de Dependências Estrangeiras:**
  - O processamento de mutações agrupa e executa em ordem correta para nunca violar constraints de chave estrangeira:
    `accounts` / `categories` ➔ `credit_cards` ➔ `credit_contracts` ➔ `transactions` ➔ `invoices` ➔ `amortization_plans` ➔ `provisions` ➔ `quotas` ➔ `sinking_funds` ➔ `scenarios` ➔ `insights` ➔ `reviews` ➔ `ml_profiles`.
- **Isolamento de Falhas por Savepoints:**
  - Falhas pontuais em uma mutação malformada não abortam o lote inteiro; o servidor rejeita apenas a mutação inválida com mensagem descritiva em `rejected_mutations` e conclui as válidas.

### 2.3. Endpoint Leve de Verificação de Mudanças (`/api/v1/sync/status`)
- **Objetivo:** Permitir que clientes Desktop e iOS façam verificações periódicas sem sobrecarregar o banco ou travar a interface do usuário.
- **Desempenho:** Resposta em < 5ms.
- **Mecanismo:**
  - Lê a revisão atual do usuário em cache em memória / header `If-None-Match`.
  - Retorna `200 OK` com `{ "latest_revision": X, "has_changes": true/false, "server_reset": false }`.
  - Se não houver alteração em relação à revisão enviada pelo cliente (`since_revision`), responde imediatamente `has_changes: false`.

### 2.4. Reset de Dados na Nuvem (`/api/v1/sync/reset`)
- Permite ao usuário solicitar a exclusão seletiva ou total de suas bases no servidor.
- **Regra Fundamental:** Quando um reset completo é realizado, o changelog do usuário é purgado e a revisão é redefinida para 0, permitindo um novo sincronismo inicial limpo sem herdar deltas de entidades antigas.

### 2.5. Armazenamento de Arquivos e Assets (`/api/v1/assets`)
- Integração compatível com S3 (MinIO no ambiente local e S3/R2 em produção).
- Fluxo de 3 etapas com URLs pré-assinadas:
  1. `POST /api/v1/assets/presigned-upload`: Gera URL temporária de escrita e ID de asset.
  2. Cliente faz upload direto do arquivo binário (extrato bancário PDF ou anexo de dívida).
  3. `POST /api/v1/assets/confirm-upload`: Confirma existência e tamanho do arquivo no bucket.
  4. `GET /api/v1/assets/:id/download-url`: Emite URL temporária de leitura para download/visualização.

---

## 3. Matriz de Endpoints da API REST

| Método | Endpoint | Protegido | Descrição |
| :--- | :--- | :---: | :--- |
| `GET` | `/docs` ou `/swagger` | Não | Swagger UI interativo da API |
| `POST` | `/api/v1/auth/register` | Não | Criação de conta |
| `POST` | `/api/v1/auth/login` | Não | Login com credenciais (retorna JWT e Refresh) |
| `POST` | `/api/v1/auth/refresh` | Não | Renovação de token de acesso |
| `POST` | `/api/v1/auth/logout` | Sim | Revogação de sessão |
| `GET` | `/api/v1/user/me` | Sim | Perfil e limites da conta |
| `GET` | `/api/v1/sync/status` | Sim | **Status Leve**: Checagem de novas mutações (<5ms) |
| `POST` | `/api/v1/sync/push-pull` | Sim | **Sync Core**: Envio e recebimento bidirecional de mutações |
| `GET` | `/api/v1/sync/delta` | Sim | Pull de mutações a partir de `since_revision` |
| `POST` | `/api/v1/sync/reset` | Sim | Reset seletivo de dados na nuvem |
| `GET` | `/api/v1/backup/export-full` | Sim | Exportação completa do backup de dados |
| `POST` | `/api/v1/backup/import-selective`| Sim | Importação seletiva de dados |
| `POST` | `/api/v1/assets/presigned-upload`| Sim | URL pré-assinada para upload de anexos |
| `POST` | `/api/v1/assets/confirm-upload` | Sim | Confirmação de upload no bucket |
| `GET` | `/api/v1/assets/:id/download-url`| Sim | URL de leitura temporária para download de anexo |

---

## 4. Comandos de Manutenção, Build e Execução

```bash
# Executar a aplicação e dependências via Docker Compose
docker-compose up -d

# Visualizar logs da API
docker-compose logs -f finsense-api

# Executar testes unitários e de integração
go test -v ./...

# Verificar tipagem e convenções do Go
go vet ./...
```
