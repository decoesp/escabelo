Desafio Técnico: Banco de Dados Key-Value para Pizzaria Bate-Papo
📋 Visão Geral
Desenvolvimento de um banco de dados key-value com persistencia com operações via TCP


🎯 Requisitos Principais
Operações via TCP

<key> ::= ([a-z] | [A-z] | [0-9] | "." | "-" | ":")+
<whitespace> ::= " "+ 
<separator> ::= "\r"
/* Qualquer coisa menos \r */
<value> ::= [a-z]
<command> ::= "read" <whitespace> <key>
            | "write" <whitespace> <key> "|" <value>
            | "delete" <whitespace> <key>
            | "status"
            | "keys"
            | "reads" <whitespace> <key>
<commands> ::= (<command> <separator>)* <command>

COMANDOS:
write -> success | error (se por algum motivo falhar)
read -> valor | error
delete -> success (se presente) | error (se a chave n existir)
status -> well going our operation
keys -> todas as chaves, separadas por \r
reads -> valores que comecem com o prefixo, separados por \r
comando inválido -> error

Tamanho maximo da key 100KB
Tamanho maximo do value ???


- 70% chaves pequenas (<= 1KB)
- 20% chaves médias (1KB - 10KB) 
- 10% chaves grandes (10KB - 100KB)
- Padrão de acesso: 80/20 (80% das consultas em 20% dos dados)



🙂 
Critérios de Avaliação
Velocidade de Escrita (prioridade alta)
Velocidade de Leitura (prioridade alta)
Tamanho do Armazenamento (prioridade média)
Persistência e Recuperação (crítico)


📊 Métricas Expandidas
| Métrica              | Descrição                               |
|----------------------|-----------------------------------------|
| Throughput Escrita   | Operações de escrita por segundo        |
| Throughput Leitura   | Operações de leitura por segundo        |
| Latência P95 Escrita | 95º percentil de latência               |
| Latência P95 Leitura | 95º percentil de latência               |
| Tempo de Recuperação | Tempo para recuperar dados após restart |


💾 Métricas de Armazenamento
| Métrica                   | Descrição                          |
|---------------------------|------------------------------------|
| Overhead de Armazenamento | <15% do tamanho dos dados          |
| Taxa de Compactação       | Eficiência na compactação de dados |
| Fragmentação              | Percentual de espaço desperdiçado  |

🛡️ Métricas de Confiabilidade
 
| Métrica                | Descrição                           |
|------------------------|-------------------------------------|
| Durabilidade dos Dados | Garantia de persistência após write |
| Consistência em Falhas | Integridade após kill -9            |
| Log de Operações       | Rastreabilidade das operações       |
 
🗓️ Cronograma de Testes
 Semana 1: Testes Básicos de Funcionalidade

Objetivo: Validar operações fundamentais e persistência básica
| Testes                       | Métricas                        |
|------------------------------|---------------------------------|
| Operações CRUD básicas       | Correção funcional              |
| Múltiplos clientes TCP       | Concorrência básica             |
| Testes de persistência       | Recuperação após restart normal |
| Dados de diferentes tamanhos | Performance variável            |
| Validação de métricas status | Precisão das métricas           |


 Semana 2: Testes de Performance e Estresse

Objetivo: Avaliar desempenho sob carga
| Testes                           | Métricas                   |
|----------------------------------|----------------------------|
| Throughput de escrita            | Ops/s, latência            |
| Throughput de leitura            | Ops/s, latência cache      |
| Carga mista (70% read/30% write) | Performance realista       |
| Testes de longa duração (4h)     | Memory leaks, estabilidade |
| Dataset grande (1M+ entries)     | Scaling horizontal         |

 Semana 3: Testes de Resiliência e Falhas

Objetivo: Garantir robustez em cenários de erro
| Testes                             | Métricas              |
|------------------------------------|-----------------------|
| Kill processo durante writes       | Integridade dos dados |
| Kill processo durante compaction   | Recuperação de estado |
| Simulação de corrupção de arquivos | Mecanismos de repair  |
| Testes de limite (storage cheio)   | Tratamento de erro    |
| Network failures simuladas         | Timeout e reconexão   |


🧪 Metodologia de Testes
 Ferramentas Recomendadas

Benchmark: redis-benchmark adaptado ou ferramenta customizada

Monitoramento: Prometheus + Grafana para métricas em tempo real

Stress Testing: Apache JMeter ou carga customizada em Go