# Chatic — Manual do Usuário

> **Idiomas:** [English](MANUAL.md) · **Português (Brasil)**
>
> Este é o **manual do usuário**: como instalar, parear seu WhatsApp, configurar a IA e usar o
> tutor no dia a dia. Para uma visão geral do projeto, veja o [README](README.pt-BR.md).

---

## Sumário

1. [O que é o Chatic](#1-o-que-é-o-chatic)
2. [Requisitos](#2-requisitos)
3. [Instalar e rodar](#3-instalar-e-rodar)
4. [Primeiro uso: o painel admin](#4-primeiro-uso-o-painel-admin)
5. [Parear o WhatsApp](#5-parear-o-whatsapp)
6. [Configurar a IA](#6-configurar-a-ia)
7. [Conversar com o tutor](#7-conversar-com-o-tutor)
8. [Comandos de chat](#8-comandos-de-chat)
9. [Modos de estudo](#9-modos-de-estudo)
10. [Grupos](#10-grupos)
11. [Links e PDFs](#11-links-e-pdfs)
12. [Mensagens de voz (áudio)](#12-mensagens-de-voz-áudio)
13. [Tarefas de administrador](#13-tarefas-de-administrador)
14. [Privacidade e seus dados](#14-privacidade-e-seus-dados)
15. [Solução de problemas](#15-solução-de-problemas)

---

## 1. O que é o Chatic

O Chatic é um **tutor de idiomas privado que vive dentro do WhatsApp**. Você mesmo o executa (num
PC, num mini-servidor ou numa VM barata na nuvem), pareia um número de WhatsApp e passa a conversar
com um tutor de IA que corrige você, ensina vocabulário e gramática, e se adapta ao seu nível e
interesses — em **qualquer** par de idiomas (ex.: um falante de português aprendendo inglês).

Há duas formas de usar:

- **Modo Família (self-chat):** cada pessoa conecta o **próprio** WhatsApp como dispositivo
  companheiro e conversa com o tutor no **próprio** chat "conversa com você mesmo", usando um
  prefixo (padrão `!`). Sem whitelist, sem admin — parear o próprio celular *é* a autorização.
- **Modo Compartilhado:** um número pareado funciona como o "bot". Outras pessoas mandam DM para
  esse número; um admin controla quem pode falar (uma whitelist). Essa conta também pode atender
  **grupos** de WhatsApp.

Você pode rodar um dos modos ou os dois ao mesmo tempo.

---

## 2. Requisitos

- **Uma máquina para rodar** — Linux, Windows ou macOS. É leve (feito para 1 vCPU / pouca RAM).
- **Um celular com WhatsApp** para parear (cada usuário pareia do próprio app).
- **Uma chave de provedor de IA** — o Chatic precisa de ao menos um LLM. Uma chave **gratuita** do
  Google Gemini já resolve (veja [§6](#6-configurar-a-ia)). OpenAI, Claude ou um Ollama local
  também funcionam.
- **FFmpeg — opcional.** Necessário só para **voz** (transcrever áudio recebido e falar respostas).
  Sem ele, o tutor de **texto** funciona normalmente; o Chatic apenas registra uma dica de
  instalação no início e desativa o áudio graciosamente.

---

## 3. Instalar e rodar

### Opção A — baixar um build de release (recomendado)

1. Baixe o pacote do seu sistema na página de Releases do projeto
   (`chatic_<versão>_<so>_<arquitetura>`).
2. Extraia. Você recebe o binário `chatic`, um `README`, `LICENSE`/`NOTICE` e um `.env.example`.
3. (Opcional) copie `.env.example` para `.env` e ajuste as configurações — **você não precisa
   colocar chaves de API aqui**; o caminho recomendado é o painel web.
4. Execute o binário:
   - Linux/macOS: `./chatic`
   - Windows: `chatic.exe`

No Debian/Ubuntu ou Fedora/RHEL você pode instalar o `.deb`/`.rpm`, que registra um serviço
`systemd` (`chatic.service`) que sobe no boot.

### Opção B — compilar do código-fonte

Requer **Go 1.20+**:

```bash
go mod tidy
go build -o chatic ./cmd/server
./chatic
```

O banco de dados é criado automaticamente no primeiro uso em `storage/` — **uma instalação nova
começa com um banco vazio e limpo** (sem contas, sem histórico).

---

## 4. Primeiro uso: o painel admin

Ao iniciar, o Chatic abre um **painel web de administração**. Por padrão:

- URL: `http://localhost:3030/admin` (mude a porta com `PORT` no `.env`).
- No primeiríssimo boot, um admin inicial é semeado a partir de `INITIAL_ADMIN_NUMBER` no `.env`
  (número de WhatsApp, só dígitos, com DDI + DDD, ex.: `5511999999999`). É usado **apenas** para
  semear o admin — depois ele vive no banco de dados.

O painel é onde você faz tudo que não deve acontecer no chat: parear contas de WhatsApp, gerenciar
chaves de API, editar o prompt do sistema, gerenciar a whitelist e apagar dados de usuário.

> **O QR Code para parear o WhatsApp aparece só no painel** — nunca no terminal/logs — por
> privacidade.

---

## 5. Parear o WhatsApp

Abra o painel → **WhatsApp Accounts** → **Add WhatsApp**. Escolha um **papel**:

- **Shared (Compartilhado)** — o número "bot" que os outros mandam DM (com whitelist, gerido por
  admin, atende grupos). Pode haver **no máximo uma** conta shared, e ela é **opcional**.
- **Personal (Pessoal)** — o WhatsApp de um membro da casa, acessado por self-chat. Sem whitelist.

Depois pareie por um dos métodos:

- **QR Code (padrão):** abra o WhatsApp no celular → **Configurações → Aparelhos conectados →
  Conectar um aparelho** → escaneie o QR mostrado no painel.
- **Código por telefone:** o painel gera um código de 8 caracteres; no WhatsApp escolha **Conectar
  com número de telefone** e digite-o.

Você pode promover/rebaixar o papel **Shared** depois, com a chave ao lado de cada conta (ligar uma
desliga a atual). Se **não** houver conta shared, o painel mostra um aviso — tudo bem: grupos e DMs
de terceiros ficam indisponíveis até você designar uma, enquanto os self-chats pessoais continuam
funcionando.

> **Instalação headless:** defina `PAIR_CODE_PHONE=<número>` no `.env` para parear uma conta shared
> por código no início, sem abrir o painel.

### Modo Família na prática

Cada membro da família abre o painel uma vez, adiciona o **próprio** WhatsApp como **Personal** e
escaneia o QR do próprio celular. A partir daí, é só abrir o chat **"conversa com você mesmo"** no
WhatsApp e começar a linha com `!` (o `SELF_CHAT_PREFIX`, configurável) para falar com o tutor. Todo
o resto que aquele dispositivo companheiro enxerga (conversas reais, grupos) é ignorado pelo Chatic.

---

## 6. Configurar a IA

O Chatic precisa de ao menos um provedor de LLM. Gerencie isso no painel em **AI Settings**.

### Conseguir uma chave gratuita (Google Gemini)

O Google Gemini tem um **plano gratuito generoso** — suficiente para uso familiar no dia a dia, sem custo.

1. Acesse **https://aistudio.google.com/apikey** e entre com uma conta Google.
2. Clique em **Create API key** (Criar chave de API) e copie (fica no formato `AIza…`).
3. No painel → **AI Settings** → cartão **Gemini** → cole a chave → **Add** (o painel é em inglês).

> **Bateu no limite diário grátis?** Adicione **várias** chaves Gemini — cada uma entra num pool
> round-robin que distribui a carga — ou rode um modelo **100% local e grátis** com o **Ollama**
> (sem nuvem, sem custo; veja o cartão do Ollama). A chave é guardada **cifrada** e nunca escrita no
> `.env` nem enviada no chat.

### Provedores, chaves e o primário

- Cada provedor (**Gemini / OpenAI / Claude**) tem a **própria lista de chaves**. A **chave nº 1 é
  a primária**; chaves extras formam um **pool round-robin** (o Chatic reveza entre elas para
  distribuir os limites de uso).
- Adicione uma chave pelo formulário do provedor; remova uma ruim pelo 🗑 ao lado dela.
- O controle **⭐ Set as primary** escolhe qual **provedor** é usado primeiro
  (`PRIMARY_LLM_PROVIDER`). Se ele falhar, o Chatic faz **failover** automático para os outros
  (Gemini → OpenAI → Claude → Ollama).
- **Ollama** (local, sem chave) usa uma URL base — defina-a no cartão do Ollama.

As chaves são **cifradas em repouso** e nunca escritas no `.env`.

### Chaves pessoais

- No chat, um usuário pode definir a própria chave com `/myai <provedor> <chave> [modelo]`
  (exclusiva dele, não entra no pool compartilhado). O bot nunca ecoa a chave e pede para você
  apagar a mensagem.
- O jeito **preferencial** é o painel (botão **🤖 IA** no usuário), para a chave nunca passar pelo
  histórico do chat.

### Prompt do sistema (opcional)

Em **AI Settings** há um **prompt do sistema** editável. Deixe **vazio** para usar o prompt de tutor
embutido do Chatic. Clique em **Load default template** para partir do embutido e customizar. Ele
aceita placeholders preenchidos por usuário: `{IdiomaAlvo}` (idioma-alvo), `{IdiomaNativo}` (idioma
nativo), `{Nivel}` (nível), `{Interesses}` (interesses), `{NomeProfessor}` (nome do professor).

---

## 7. Conversar com o tutor

Na primeira mensagem ao tutor, ele roda um breve **onboarding**:

1. Seu nome
2. Ano de nascimento
3. **Idioma nativo** (seu idioma de apoio, ex.: português)
4. **Idioma-alvo** (o que você quer aprender, ex.: inglês)
5. **Nível** — faça um teste rápido de 3 perguntas, ou pule e comece no A1
6. **Nome do professor** — como você quer chamar seu tutor (ele responderá por esse nome)
7. **Interesses / hobbies** — usados para guiar os temas de conversa

Depois disso, é só **conversar naturalmente no seu idioma-alvo**. O tutor responde no contexto e,
quando você erra, acrescenta uma breve correção **💡 Quick Tip** sem interromper a conversa. Você
ganha **XP** ao praticar (veja `/ranking`).

Digite `/restart` para refazer o onboarding, ou `/language` para revisar/ajustar seus idiomas.

---

## 8. Comandos de chat

Todos os comandos começam com `/`. Os universais funcionam num DM/self-chat:

| Comando | O que faz |
|---|---|
| `/help` | Lista os comandos disponíveis |
| `/restart` | Reinicia o onboarding |
| `/language` | Revisar / ajustar seu idioma nativo e alvo |
| `/tips` | Sugere o que você poderia dizer em seguida, no idioma-alvo (scaffolding) |
| `/ranking` | Mostra seu XP / progresso |
| `/myai <provedor> <chave> [modelo]` | Define sua chave de IA **pessoal** (veja [§6](#6-configurar-a-ia)) |
| `/forget` | **Apaga todos os seus dados** (dois passos: pede confirmação com `/forget CONFIRM`) |

Os modos de estudo e comandos de grupo estão nas próximas seções.

---

## 9. Modos de estudo

Prática focada, sob demanda (cada um é uma única resposta, além das correções que já acontecem
naturalmente na conversa):

| Comando | O que faz |
|---|---|
| `/grammar <tópico>` | Explica uma regra gramatical no seu idioma **nativo**, com exemplos |
| `/word` | Ensina uma palavra/expressão útil para seu nível e interesses |
| `/vocab <tema>` | Uma lista de vocabulário temática (com traduções e exemplos) |
| `/quiz` | Um quiz curto baseado na sua conversa recente (respostas ao final) |
| `/fix <frase>` | Corrige uma frase e explica o porquê (`/fix` sozinho corrige sua última mensagem) |

---

## 10. Grupos

O Chatic pode entrar em **grupos** de WhatsApp (atendidos só pela conta **shared**).

**Configuração**

| Comando | O que faz |
|---|---|
| `/newgroup` | Cria um grupo de estudo e te dá um código de entrada |
| `/join <código>` | Entra num grupo de estudo existente |
| `/groupai <código> <provedor> <chave> [modelo]` | Define uma chave de IA **compartilhada** do grupo |

**Dentro de um grupo** o tutor fica quieto, a menos que convidado:

- **Fase 1 (reativo):** `/ask <pergunta>`, `/correct <texto>`, ou **@mencione** o bot.
- **Fase 2 (atividades):**

| Comando | O que faz |
|---|---|
| `/gquiz [tema]` | Posta um quiz em **enquete** nativa do WhatsApp |
| `/greveal` | Revela a resposta + explicação do último `/gquiz` |
| `/gword` | Palavra do dia para o grupo |
| `/gchallenge [tema]` | Um desafio rápido em grupo |
| `/ghelp` | Lista os comandos de grupo |

Grupos têm limite de uso para manter o custo de IA sob controle.

---

## 11. Links e PDFs

- **Envie um link** (qualquer URL `http(s)`) num DM e o Chatic baixa a página, extrai o texto
  legível e discute o **conteúdo real** com você, em vez de chutar a partir da URL.
- **Envie um PDF** e o Chatic extrai o texto (primeiras páginas) para você estudar a partir dele. Só
  PDFs são suportados; PDFs escaneados/só-imagem não geram texto (sem OCR).

Por segurança, páginas baixadas e arquivos enviados são sempre tratados como **material de
referência, nunca instruções** (proteção contra injeção de prompt), e links inacessíveis falham
graciosamente.

---

## 12. Mensagens de voz (áudio)

Se o **FFmpeg** estiver instalado:

- **Envie um áudio** e o Chatic transcreve, tutora em cima dele e pode responder.
- As respostas podem ser faladas de volta em áudio (texto-para-fala).

Sem FFmpeg, enviar um áudio num DM recebe uma resposta amigável ("voz desativada, siga digitando") —
o tutor de texto não é afetado. Arquivos temporários de áudio são apagados imediatamente após o uso.

---

## 13. Tarefas de administrador

Para a conta **shared**, um admin gerencia quem pode falar. Do próprio chat do admin:

| Comando | O que faz |
|---|---|
| `/list` | Lista os usuários na whitelist |
| `/add <número>` | Autoriza um número de WhatsApp (só dígitos, com DDI/DDD) |
| `/delete <número>` | **Apaga** os dados daquele usuário e o remove |
| `/recover <número>` | Restaura um usuário removido anteriormente |

O painel espelha esses comandos (lista de usuários, adicionar, apagar) além do gerenciamento de
chaves de API e do prompt do sistema. Remetentes desconhecidos para a conta shared são descartados
**antes** de qualquer processamento.

---

## 14. Privacidade e seus dados

Privacidade é o propósito do Chatic. Garantias principais:

- **Conteúdo de mensagem nunca é logado** no console — apenas metadados (como um id de usuário).
- **O histórico de conversa é cifrado em repouso** no banco de dados local.
- **As chaves de API são cifradas em repouso** e nunca escritas no `.env`.
- **Você executa** — seus dados ficam na sua máquina; nada é vendido ou enviado a terceiros além da
  chamada ao provedor de IA necessária para gerar a resposta.
- **Direito ao esquecimento (LGPD):** `/forget` (depois `/forget CONFIRM`) apaga **todos** os seus
  dados — perfil, mensagens e participações em grupo. Um admin também pode apagar um usuário com
  `/delete`, e o painel tem um botão de apagar usuário. O apagamento é físico de verdade, não uma
  ocultação.

Um **build de release sempre sai com um banco de dados limpo e vazio** — nenhuma conta pareada ou
dado pessoal é jamais empacotado no pacote distribuído.

---

## 15. Solução de problemas

**O painel não abre / porta em uso.** Outro processo está segurando a porta. Mude `PORT` no `.env`,
ou pare a outra instância.

**Nenhum QR aparece ao adicionar uma conta.** Espere um instante (o painel fica buscando) e
confirme que escolheu um papel. Se disser expirado, feche e reabra **Add WhatsApp** para iniciar um
novo pareamento.

**Aviso "sem conta shared".** Esperado se você pareou só dispositivos pessoais. Grupos e DMs de
terceiros precisam de uma conta shared; designe uma com a chave **Shared**. Os self-chats pessoais
funcionam de qualquer forma.

**O tutor não responde.** Verifique **AI Settings**: você precisa de ao menos uma chave funcionando.
Se o provedor primário estiver fora do ar ou sem cota, o Chatic faz failover — mas se **todos** os
provedores falharem, você recebe um erro. Adicione/substitua uma chave com os controles 🗑 / **Add**.

**Áudio não funciona.** Instale o **FFmpeg** e reinicie. O Chatic registra uma dica de instalação
específica do sistema no início quando ele está ausente.

**Uma mensagem não foi respondida na conta shared.** Só números na whitelist são atendidos; adicione
o número com `/add` ou pelo painel.

**Dispositivo pessoal não responde.** Confirme que você está mandando mensagem para **você mesmo** (o
chat "conversa com você mesmo") e que sua linha começa com o prefixo (`!` por padrão).
