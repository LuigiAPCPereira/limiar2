## 2025-06-09 - [File Permissions] Forçar criação do banco SQLite/Tursogo com 0600
**Vulnerabilidade:** O arquivo de banco de dados (`.db`) que hospeda a sessão MTProto do Telegram e as mensagens em texto plano estava sendo criado com o umask padrão do sistema (que normalmente resulta em permissões `0644`), permitindo que outros usuários locais no mesmo sistema acessassem os dados críticos.
**Aprendizado:** A criação automática de arquivos pelo driver `database/sql` para bancos de dados baseados em arquivos pode não aplicar regras de segurança estritas, deixando os arquivos resultantes vulneráveis a leituras não autorizadas.
**Prevenção:** Antes de chamar `sql.Open(...)`, deve-se criar manualmente o arquivo do banco de dados (se não existir) utilizando `os.OpenFile` com permissões `0600` e, caso exista, aplicar `os.Chmod` para garantir que as permissões restritas (owner-only) continuam sendo aplicadas. Isso assegura que dados sensíveis armazenados localmente sejam fortemente protegidos em nível de sistema de arquivos.

## 2025-06-10 - [Log Injection] Prevenção de Log Injection e MTProto Injection via input de username
**Vulnerabilidade:** A função `NormalizeUsername` extraía substrings do username fornecido pelo usuário via CLI (ex: `channels add <username>`) mas não sanitizava o conteúdo real. Uma entrada como `@username\nAdmin` ou com parâmetros web como `?start=x` poderia causar vazamentos nos logs de stdout (Log Injection) e requisições malformadas na API do Telegram (SSRF / MTProto Injection).
**Aprendizado:** Todo input advindo do usuário, especialmente aqueles que serão enviados a APIs externas ou logados em disco/stdout, deve passar por validação estrita (allowlist de caracteres esperados). Neste caso, a regex `^[a-zA-Z0-9_]+$` protege contra payloads indesejados.
**Prevenção:** Antes de utilizar e logar variáveis como `username`, aplique sempre a sanitização rigorosa — como a remoção de control characters (`\n`, `\r`) e fallback seguro validando em loop ou regex. Além disso, identificou-se que `d.log.Error("💥 Panic no handler recuperado", "panic", r)` no Dispatcher expõe o objeto bruto do panic `r`, que no futuro deve ser mitigado.

## 2025-06-10 - [Segurança] Panic Recovery vazando dados sensíveis no log
**Vulnerabilidade:** O panic recovery no `Dispatcher` (`internal/telegram/dispatcher.go`) estava logando o objeto bruto de panic (`r`) diretamente via `d.log.Error(..., "panic", r)`. Isso poderia vazar dados altamente sensíveis da memória, como o MTProto session token ou conteúdos de mensagens, caso um erro inesperado expusesse essa variável durante o processo na goroutine.
**Aprendizado:** Logar o tipo (`%T`) perde informações de depuração essenciais. É mais apropriado envelopar a mensagem usando `fmt.Errorf("panic: %v", r)`. Assim, extrai-se a mensagem literal sem utilizar a reflexão sobre o objeto, o que previne dump de structs com segredos embutidos, ao mesmo tempo que mantém informações valiosas sobre o erro ocorrido.
**Prevenção:** Nunca logue o objeto bruto do `recover()` (`interface{}`) como valor direto em chaves de log (que utilizam reflexão). Em vez disso, converta-o para um erro com `fmt.Errorf("panic: %v", r)`.

## 2025-06-12 - [Information Leakage] Erros brutos expostos na API HTTP
**Vulnerabilidade:** A API do dashboard estava expondo erros internos detalhados (ex: `err.Error()`) nas respostas HTTP usando `http.Error()`, vazando informações sensíveis sobre o estado do sistema ou caminhos internos.
**Aprendizado:** A exposição de raw internal errors a clientes não autenticados pode facilitar a enumeração do sistema.
**Prevenção:** Sempre utilize respostas de erro genéricas como `http.StatusText` ao cliente e registre o erro original completo apenas no log interno.
## 2025-02-27 - Permissões do Banco de Dados (.db)
**Vulnerabilidade:** O arquivo do banco de dados (que armazena o token MTProto) estava sendo criado usando uma sintaxe octal antiga (0600) em vez da moderna (0o600). Adicionalmente, dependia apenas do `os.OpenFile` na criação e ignorava a correção de arquivos já criados com umask menos restritivo.
**Aprendizado:** Bancos criados antes da imposição estrita poderiam estar com permissões brandas e legíveis globalmente. E a umask do sistema pode contornar os parâmetros da criação.
**Prevenção:** Sempre usar a notação moderna do Go `0o600` e impor ativamente com `os.Chmod` na inicialização para garantir a segurança independentemente da umask e mitigar histórico.
