# Benchmark Tools

Este diretório contém duas ferramentas de benchmark:

## 1. main.go - Benchmark Completo

Ferramenta de benchmark completa com workload realista 80/20.

### Características:
- Padrão de acesso 80/20 (80% das requisições em 20% das chaves)
- Distribuição de tamanho de chaves (70% pequenas, 20% médias, 10% grandes)
- Pré-população de dados
- Métricas detalhadas de latência e throughput
- Duração configurável

### Uso:
```bash
# Via Makefile
make bench                # Padrão: 30s, 10 clientes, 80% leitura
make bench-intensive      # Intensivo: 60s, 50 clientes
make bench-write         # Foco em escrita: 20% leitura

# Direto
./bin/bench \
  -addr=localhost:8080 \
  -duration=30s \
  -concurrency=10 \
  -read-ratio=0.8 \
  -key-count=10000
```

### Flags:
- `-addr`: Endereço do servidor (default: localhost:8080)
- `-duration`: Duração do teste (default: 30s)
- `-concurrency`: Número de clientes concorrentes (default: 10)
- `-read-ratio`: Proporção de leituras 0.0-1.0 (default: 0.8)
- `-key-count`: Número total de chaves únicas (default: 10000)
- `-hot-key-ratio`: Proporção de chaves "quentes" (default: 0.2)

## 2. test.go - Teste Simples

Ferramenta de teste mais simples e direta, focada em modos específicos.

### Características:
- Três modos: write, read, mixed
- Estatísticas com percentis (P50, P95, P99)
- Distribuição de tamanho de valores (70/20/10)
- Padrão 80/20 para leituras
- Mais leve e rápido

### Uso:

#### Via Makefile (Recomendado):

```bash
# Teste de escrita (10k operações, 10 workers)
make test-write

# Teste de leitura (10k operações, 10 workers)
make test-read

# Teste misto - 70% leitura, 30% escrita
make test-mixed

# Executar todos os testes em sequência
make test-all
```

#### Uso Direto:

```bash
# Compilar
go build -o bin/test ./cmd/bench/test.go

# Teste de escrita
./bin/test -mode=write -ops=10000 -c=10

# Teste de leitura
./bin/test -mode=read -ops=10000 -c=10

# Teste misto
./bin/test -mode=mixed -ops=10000 -c=10
```

### Flags:
- `-mode`: Modo do teste: write, read, ou mixed (default: write)
- `-ops`: Número de operações a realizar (default: 10000)
- `-c`: Número de workers concorrentes (default: 10)

### Variável de Ambiente:
- `SERVER_ADDR`: Endereço do servidor (default: 127.0.0.1:8080)

```bash
# Usar servidor em porta diferente
SERVER_ADDR=localhost:9090 ./bin/test -mode=write -ops=5000
```

## Comparação

| Característica | main.go (bench) | test.go (test) |
|----------------|-----------------|----------------|
| Complexidade | Alta | Baixa |
| Pré-população | Sim | Não |
| Duração | Baseada em tempo | Baseada em operações |
| Modos | Configurável via ratio | 3 modos fixos |
| Métricas | Throughput + Latência | Throughput + Percentis |
| Uso | Benchmark completo | Testes rápidos |

## Workflow Recomendado

### 1. Desenvolvimento - Testes Rápidos
```bash
# Terminal 1: Servidor
make run

# Terminal 2: Testes rápidos
make test-write    # Testar escritas
make test-read     # Testar leituras
make test-mixed    # Testar workload misto
```

### 2. Validação - Benchmark Completo
```bash
# Terminal 1: Servidor
make run

# Terminal 2: Benchmark completo
make bench         # 30 segundos
make bench-intensive  # 60 segundos, carga pesada
```

### 3. Análise de Performance
```bash
# Teste de escrita pura
make test-write

# Aguardar compactação (alguns minutos)

# Teste de leitura após compactação
make test-read

# Comparar latências
```

## Exemplos de Saída

### test.go Output:
```
=== Benchmark Results ===
Total Operations:    10000
Successful:          9998
Failed:              2
Duration:            12.45s
Throughput:          803.21 ops/sec
P50 Latency:         8.234ms
P95 Latency:         15.678ms
P99 Latency:         23.456ms
Max Latency:         45.123ms

=== Server Status ===
well going our operation
writes=5000 reads=5000 deletes=0 flushes=2 memtable_size=1234567 sst_count=3 wal_size=456789
```

### main.go Output:
```
============================================================
BENCHMARK RESULTS
============================================================

Operations:
  Total Operations: 45678
  Reads:            36542 (80.0%)
  Writes:           8201 (18.0%)
  Deletes:          935 (2.0%)
  Errors:           0

Throughput:
  Total:            1522.60 ops/sec
  Reads:            1218.07 ops/sec
  Writes:           273.37 ops/sec

Latency (Average):
  Read:             6.5ms
  Write:            8.2ms
============================================================
```

## Dicas

1. **Sempre inicie o servidor antes** de rodar os testes
2. **Use test.go** para testes rápidos durante desenvolvimento
3. **Use main.go** para benchmarks completos e análise de performance
4. **Execute test-all** para validação rápida de todas as operações
5. **Monitore o status** do servidor após cada teste

## Troubleshooting

### Connection Refused
```bash
# Verificar se o servidor está rodando
ps aux | grep escabelo

# Iniciar o servidor
make run
```

### Latências Altas
```bash
# Verificar número de SST files
echo "status\r" | nc localhost 8080

# Se muitos SSTs, aguardar compactação ou ajustar intervalo
./bin/escabelo -compaction-interval=2m
```

### Erros de Timeout
```bash
# Reduzir concorrência
./bin/test -mode=write -ops=5000 -c=5

# Ou aumentar timeout no código (se necessário)
```
