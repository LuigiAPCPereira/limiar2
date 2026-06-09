## 2024-06-09 - Acessibilidade básica no Dashboard Vanilla
**Aprendizado:** O dashboard utiliza `<div>` como elementos interativos (cards de canal e itens de mensagem) sem semântica ou suporte a teclado, além de omitir ARIA labels em botões e live regions na lista SSE.
**Ação:** Em componentes vanilla, sempre que um `<div>` for clicável, deve receber `role="button"`, `tabindex="0"`, event handler para teclado (`onkeydown`), e estilo de `:focus-visible`.
