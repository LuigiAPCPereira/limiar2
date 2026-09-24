# Política de toolchain e dependências — Current Stable First

**Origem:** direção explícita do mantenedor em 2026-09-23.  
**Escopo:** Limiar 3.0 e novas capacidades reconstruídas.  
**Autoridade:** direção do mantenedor; não substitui ADR Accepted específico nem remove gates de segurança/compatibilidade.

## Regra

Para uma fundação nova, a versão **stable atual** de linguagem, toolchain, biblioteca ou protocolo é o default de avaliação e adoção.

Uma versão anterior só deve ser escolhida quando houver razão técnica concreta, documentada e demonstrável, como:

- regressão;
- incompatibilidade real;
- instabilidade;
- requisito não suportado;
- vulnerabilidade/regressão na versão nova;
- dependência estrutural ainda incompatível;
- custo de migração desproporcional demonstrado.

“Já usamos”, “já investigamos” ou “é mais conhecida” não são, sozinhos, motivos suficientes para permanecer numa versão inferior.

## Processo

Para dependência/toolchain estrutural:

1. identificar a versão stable atual;
2. consultar release notes, changelog e security notes;
3. verificar breaking changes/compatibilidade;
4. verificar issues relevantes abertas e limitações conhecidas;
5. executar os gates adequados;
6. piná-la explicitamente;
7. registrar eventual exceção se uma versão anterior for mantida.

Não usar `latest` dinamicamente em produção/CI. “Current Stable First” define o baseline de escolha, não a ausência de pinning.

## Features novas

Adotar a plataforma moderna por inteiro; adotar suas features **seletivamente**.

Uma feature nova deve entrar quando ajuda um requisito real de:

- correção;
- segurança;
- performance;
- observabilidade;
- manutenção;
- simplicidade.

Não usar feature experimental, abstração, otimização ou mecanismo novo apenas para parecer moderno.

## Performance

Antes de otimização própria, aproveitar corretamente:

- melhorias do compilador/runtime atual;
- GC/allocator atuais;
- profiling/diagnostics atuais;
- standard library atual;
- capabilities nativas das dependências;
- melhorias de protocolo/SDK já disponíveis.

Somente depois de workload e profiling considerar PGO, pooling customizado, scheduler próprio, zero-copy, unsafe ou mecanismos equivalentes.

## Go

Na data desta decisão, Go 1.27.1 é o baseline preferencial a ser verificado para L3-002. Go 1.26.x passa a ser fallback somente se incompatibilidade/regressão concreta for demonstrada.

A Evidence de L3-002 deve ser produzida no toolchain que pretendemos usar, evitando validar toda a fundação numa versão antiga para depois trocar o runtime.

## gotd

A mesma regra vale para `github.com/gotd/td`.

A versão stable atual deve ser auditada e testada como baseline preferencial. A versão v0.161.0 continua valiosa porque foi investigada profundamente em L3-001A, mas isso não a torna automaticamente a versão a ser usada por L3-002.

Antes de pinning final:

- comparar stable atual vs v0.161.0;
- revisar schema/layer;
- auth/session/runtime;
- fixes e regressões;
- open issues relevantes;
- build/test/race;
- experimento de lifecycle na versão escolhida.

## Regra final

> O presente é o default; o legado precisa justificar por que devemos ficar para trás.

Esta política não autoriza upgrade automático. Ela define como a decisão deve ser tomada.
