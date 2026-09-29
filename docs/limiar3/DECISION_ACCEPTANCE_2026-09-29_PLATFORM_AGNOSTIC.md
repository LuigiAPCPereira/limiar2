# Registro de aceitação — L3 ADR 005 / portabilidade e neutralidade de plataforma

**Data local do mantenedor:** 2026-09-29  
**Escopo:** Limiar 3.0 — correção da restrição Linux introduzida na fundação L3-002/L3-003.  
**Decision a formalizar:** `docs/limiar3/adr/005-platform-agnostic-runtime-and-storage.md`.

## Correção explícita do mantenedor

Na conversa do projeto, o mantenedor esclareceu que o Limiar não é um produto Linux-only e que a reconstrução Limiar 3 não deve introduzir uma identidade de plataforma que não existia nas gerações anteriores.

A orientação explícita é:

- o Limiar permanece agnóstico ao sistema operacional por padrão;
- não há requisito de produto para denominar runtime, Telegram, MCP, query, bootstrap, observabilidade ou ownership por sistema operacional;
- a persistência de sessão/credencial também não deve se tornar Linux-only por conveniência da implementação;
- detalhes realmente específicos de plataforma só podem existir no menor adapter necessário e mediante necessidade técnica comprovada;
- a ocorrência de um primeiro ambiente de teste/deploy em Linux não transforma o contrato do produto em Linux-only;
- os build tags `//go:build linux` introduzidos de forma ampla na L3 devem ser removidos do código que é portável;
- comentários, mensagens operacionais, erros destinados a pessoas e documentação criada/alterada nesta correção devem usar pt-BR, preservando identificadores e contratos técnicos externos.

## Evidência de continuidade do produto

A comparação com as gerações anteriores mostra que essa correção restaura continuidade em vez de introduzir uma arquitetura nova:

- o Limiar anterior possuía `FileSessionStorage` baseado em `os.ReadFile`, `os.WriteFile` e `os.Stat`, sem build tag de Linux;
- o mesmo Limiar também persistia sessão via banco implementando `session.Storage`;
- o Limiar 2 usava `TursoSessionStorage` sobre `storage.Repository`, também sem dependência de plataforma;
- o histórico do repositório contém validação nativa de SQLite no Windows, portanto o projeto não tinha premissa histórica Linux-only.

## Efeito sobre decisões anteriores

Esta aceitação corrige apenas a dimensão de plataforma de L3 ADR 001 e L3 ADR 002.

Continuam preservados, salvo conflito explícito com esta decisão:

- separação da credencial Telegram de Evidence;
- fail-closed para sessão ausente/incompatível/revogada;
- ownership por `TelegramAuthorizationIdentity`;
- bootstrap administrativo explícito;
- semantic readiness;
- contrato `TelegramQuery`;
- limites de concorrência, paginação e observabilidade segura;
- boundary MCP read-only definido no L3 ADR 004.

A formulação "Linux/single-host/single-process" não deve mais ser usada como justificativa para restringir código portável ao Linux.

## Limites da aceitação

Esta autorização permite corrigir os ADRs/documentação derivados e implementar a remoção dos build tags/sufixos Linux do código portável.

Ela não autoriza:

- login Telegram real, OTP/2FA ou uso de credenciais reais;
- exposição pública do MCP;
- criação de tunnel real;
- merge/deploy;
- reescrita ampla de identificadores técnicos apenas para tradução;
- alteração de contratos externos do gotd ou MCP.

## Referência

Este registro fornece a referência durável para a aceitação do L3 ADR 005. O ADR deve apontar para o commit que criou este registro.
