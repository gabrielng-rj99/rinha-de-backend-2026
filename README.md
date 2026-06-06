# Rinha de Backend 2026 — Fraud Detection API

Submissão para a [Rinha de Backend 2026](https://github.com/zanfranceschi/rinha-de-backend-2026).

## Stack

- **Linguagem**: Go 1.22
- **Algoritmo**: IVF (Inverted File Index) + KD-Tree
- **Indexação**: K-Means (512 centroids) com build offline
- **Comunicação**: Unix socket + SCM_RIGHTS fd-passing (zero cópia extra)
- **HTTP**: Parser hand-rolled (zero alocação no hot path)

## Arquitetura

```
┌─────────┐       Unix Socket        ┌─────────┐
│   LB    │ ──── SCM_RIGHTS fd ────→ │  API 1  │
│ (Go)    │                          │  (Go)   │
│         │ ──── SCM_RIGHTS fd ────→ │         │
│ :9999   │                          └─────────┘
└─────────┘                          ┌─────────┐
                                     │  API 2  │
                                     │  (Go)   │
                                     └─────────┘
```

- **Load Balancer**: Aceita conexões TCP na porta 9999, distribui via round-robin
- **API Instances**: Recebem file descriptors via Unix socket, servem HTTP direto no fd
- **Index**: Pré-construído no build do Docker (K-Means → IVF → KD-Tree por partição)

## Build & Run

```bash
docker compose up --build -d
```

O builder roda automaticamente durante o build da imagem:
1. Baixa `references.json.gz` (3M vetores)
2. Executa K-Means (512 centroids, 5 iterações)
3. Constrói KD-Trees por partição
4. Salva índice binário (`references.idx`)

## Detecção

Cada transação é normalizada em um vetor de 14 dimensões e classificada via 5-NN no índice de referência. Se ≥3 dos 5 vizinhos são fraude (score ≥ 0.6), a transação é negada.

## Resultado

| Métrica | Valor |
|---------|-------|
| Score final | 5736.72 / 6000 |
| Detection | 3000/3000 (perfeito) |
| p99 latency | 1.83ms |
| FP / FN / Errors | 0 / 0 / 0 |

## Licença

MIT
