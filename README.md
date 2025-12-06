# Echo App

Uma aplicação web simples em Go que ecoa mensagens.

## 🚀 Funcionalidades

- Página inicial com formulário para inserir mensagem
- Página de resultado que exibe a mensagem enviada
- Endpoint de health check (`/health`)
- Container Docker otimizado (multi-stage build)

## 📁 Estrutura

```
.
├── main.go                 # Código principal da aplicação
├── go.mod                  # Dependências Go
├── Dockerfile              # Build da imagem Docker
├── templates/
│   ├── index.html          # Página inicial
│   └── echo.html           # Página de resultado
└── .github/
    └── workflows/
        └── build-push-ecr.yml  # CI/CD para ECR
```

## 🛠️ Executar Localmente

### Sem Docker

```bash
go run main.go
```

### Com Docker

```bash
docker build -t echo-app .
docker run -p 8080:8080 echo-app
```

Acesse: http://localhost:8080

## 🔧 Variáveis de Ambiente

| Variável | Descrição | Default |
|----------|-----------|---------|
| `PORT` | Porta do servidor | `8080` |

## 📦 CI/CD

O workflow do GitHub Actions:

1. Executa testes
2. Faz build da imagem Docker
3. Push para Amazon ECR com tags:
   - `sha` (commit hash)
   - `branch` (nome do branch)
   - `latest`
4. Scan de vulnerabilidades com Trivy

### Secrets necessários

Configure no repositório:

- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`

## 🔒 Segurança

- Imagem baseada em Alpine Linux
- Executa como usuário não-root
- Health check configurado
- Scan de vulnerabilidades automático
