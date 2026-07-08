# Chatic — Tutor de Idiomas Privado Multilíngue (WhatsApp Bot)

[English](README.md) · **Português (BR)**

Chatic é um ecossistema privado de aprendizado de idiomas via WhatsApp, desenvolvido em Go (Golang) e projetado para rodar com eficiência extrema em hardware modesto (como instâncias gratuitas da Oracle Cloud com 1 Core CPU e 6 GB RAM).

O sistema suporta a prática de **qualquer idioma de escolha do aluno** (ex: Inglês, Espanhol, Francês, Alemão, Japonês) a partir do seu **idioma nativo** (ex: Português, Espanhol, Inglês), oferecendo prática conversacional (texto e áudio), gamificação (XP e rankings familiares) e proteção contra desperdício de tokens (failover de IA).

> 📖 **Novo por aqui? Leia o [Manual do Usuário](MANUAL.pt-BR.md)** — um guia passo a passo para instalar, parear o WhatsApp, configurar a IA e usar cada comando.

---

## Começo Rápido (sem mexer no `.env`)

Você **não precisa** tocar no `.env` para rodar — tudo é configurado no painel web:

1. **Instale** o Chatic (veja **Instalação** abaixo) e inicie.
2. Abra **http://localhost:3030/admin** e crie a senha do painel (primeiro acesso).
3. Adicione uma chave de IA **grátis** (veja a próxima seção) em **⚙️ AI Settings** e salve.
4. Abra a aba **Pareamento** e **escaneie o QR** com o WhatsApp.
5. Cadastre você / sua família em **Usuários** (ou ative o multi-conta para cada um parear o próprio WhatsApp).

Pronto — é só conversar com o tutor no WhatsApp. O arquivo `.env` é **opcional**, só para ajustes avançados/de host.

---

## Chave de IA Grátis (Google AI Studio) — é grátis 🎉

O Chatic funciona muito bem com o **Google Gemini**, que tem um **plano gratuito generoso** — suficiente para uso familiar no dia a dia **sem custo**.

1. Acesse **https://aistudio.google.com/apikey** e entre com uma conta Google.
2. Clique em **Create API key** (Criar chave de API) e copie (fica no formato `AIza…`).
3. No painel (**/admin → ⚙️ AI Settings**), cole no campo **Google Gemini API Key** e **Salve** (o painel é em inglês).
   - A chave é guardada **cifrada** no banco — **nunca** coloque no `.env` nem envie no chat.
4. Pronto — o tutor já começa a usar.

**Como funciona:** cada mensagem que você envia é encaminhada ao provedor de IA escolhido (Gemini por padrão) junto com um prompt de professor; o bot tem **failover** automático (Gemini → OpenAI → Claude → Ollama), então uma queda ou limite de cota de um provedor não interrompe a aula. Bateu no limite diário grátis? Adicione **várias** chaves Gemini (separadas por vírgula) para um pool round-robin, ou rode um modelo **100% local e grátis** com o **Ollama** (veja abaixo) — sem nuvem, sem custo.

---

## Configuração do Ambiente (.env) — opcional

O `.env` é **opcional**: o Chatic sobe com padrões sensatos e você configura as chaves e usuários pelo painel web. Edite o `.env` só para ajustes avançados/de host (porta, timeouts, guarda de idade de mensagem, multi-conta). Se for criar um, use o modelo a seguir.

> 🔒 **Segredos não ficam no `.env`.** As chaves de API e demais dados sensíveis são configurados pelo **painel web** (`/admin`) e guardados **cifrados (AES‑256‑GCM)** no banco SQLite, protegidos pela chave‑mestra local (`storage/.masterkey`, `0600`) — nunca em texto puro no disco. O `.env` guarda apenas configurações **não sensíveis**; o próprio bot reescreve o arquivo sem chaves ao salvar as configurações no painel. Você pode, opcionalmente, colocar uma chave no `.env` só para o *bootstrap* inicial — ela é migrada para o cofre cifrado assim que você salva no painel.

```env
# Configurações Gerais
PORT=3030
ENV=development

# Self-Chat: conversar com o bot no seu próprio número ("Mensagens para você mesmo")
# Mensagens iniciadas com este prefixo são processadas pelo tutor. Deixe vazio para desativar.
SELF_CHAT_PREFIX=!

# Provedor principal de LLM (opções: gemini, openai, claude, ollama)
PRIMARY_LLM_PROVIDER=gemini

# Chaves de API: NÃO ficam aqui. Configure pelo painel web (/admin) — são
# guardadas cifradas (AES-GCM) no SQLite, não no .env. As linhas abaixo são
# apenas para bootstrap opcional e migram para o cofre cifrado ao salvar no painel.
# GEMINI_API_KEY=
# OPENAI_API_KEY=
# CLAUDE_API_KEY=

# Configuração de LLM Local (Ollama)
OLLAMA_API_BASE=http://localhost:11434
OLLAMA_MODEL=llama3.2

# Limites e Timeouts
LLM_TIMEOUT_SECONDS=10

# Idade máxima (em segundos) de uma mensagem recebida que ainda será processada.
# Ao reconectar após ficar offline, o WhatsApp reentrega toda a fila de mensagens
# acumuladas; sem este limite o bot responderia a todas de uma vez, inundando os chats.
# Mensagens mais antigas que isto são ignoradas. Use 0 para desativar. (padrão: 300)
MAX_MESSAGE_AGE_SECONDS=300

# Banco de Dados
DATABASE_PATH=storage/tutor.db

# Número do admin no WhatsApp — usado APENAS no primeiro boot para semear o admin.
# É dado pessoal: depois de criado, o admin vive no banco (tabela de usuários) e o
# bot NÃO reescreve este número no .env. Preencha só para o bootstrap inicial.
# Só dígitos, com DDI/DDD.
INITIAL_ADMIN_NUMBER=

# Pareamento por código de telefone (opcional): em vez de escanear o QR no painel,
# gere um código de 8 dígitos para digitar no WhatsApp. Deixe vazio para usar o QR.
PAIR_CODE_PHONE=

# Modo multi-conta (opcional): permite que cada pessoa da casa conecte o PRÓPRIO
# WhatsApp e converse com o tutor por self-chat, sem whitelist nem admin.
# true = habilita parear novas contas pessoais pelo painel. (padrão: false)
MULTI_ACCOUNT_ENABLED=false
```

---

## Integração de IA e LLM Local (Ollama)

Além de provedores de nuvem (Gemini, OpenAI, Claude), você pode rodar o bot **totalmente local e offline** integrando o **Ollama**:

1.  **Instale o Ollama**: Baixe e instale em [ollama.com](https://ollama.com/) no seu host (Windows, Mac ou Linux).
2.  **Baixe o Modelo Desejado**: Execute no terminal:
    ```bash
    ollama pull llama3.2
    ```
3.  **Habilite no Bot**: No arquivo `.env`, altere:
    ```env
    PRIMARY_LLM_PROVIDER=ollama
    OLLAMA_API_BASE=http://localhost:11434  # Ou o IP do Windows se rodando o bot de dentro do WSL (ex: http://172.x.x.x:11434)
    OLLAMA_MODEL=llama3.2
    ```

---

## Customização e Edição do Agente (Prompts)

Você tem duas formas de editar a personalidade, tom de fala e comportamento didático do bot:

### Opção A: Sem recompilar (Via `.env`)
Você pode editar a variável `CUSTOM_SYSTEM_PROMPT` no `.env`. O bot substitui dinamicamente as tags abaixo baseando-se no perfil de quem está conversando:
-   `{IdiomaAlvo}`: O idioma que o aluno quer aprender (ex: Inglês).
-   `{IdiomaNativo}`: A língua materna do aluno (ex: Português).
-   `{Nivel}`: O nível de proficiência CEFR (A1 a C2).
-   `{Interesses}`: Hobbies e temas que o aluno gosta.
-   `{NomeProfessor}`: Nome que o aluno escolheu para a professora durante o onboarding.

> Os tokens de placeholder ficam em português de propósito — são um contrato de template voltado ao usuário.

### Opção B: Recompilando o Código
Você pode editar diretamente a lógica e o prompt padrão estruturado em `internal/tutor/engine.go`.

---

## Instalação

Escolha o método que preferir. O tutor por **texto** roda com **zero dependências** de sistema; o **FFmpeg** é opcional (apenas para áudio, e os instaladores tentam configurá-lo para você).

> Após instalar, abra **http://localhost:3030/admin**, crie a senha do painel (primeiro acesso) e **escaneie o QR Code** para parear o WhatsApp — o QR aparece no próprio painel, não no terminal.

### Linux / macOS — uma linha
```bash
curl -fsSL https://raw.githubusercontent.com/jousudo/chatic/main/install.sh | sh
```

### Windows (PowerShell) — uma linha
```powershell
irm https://raw.githubusercontent.com/jousudo/chatic/main/install.ps1 | iex
```

### Linux com serviço (auto-start no boot) — pacote .deb / .rpm
Baixe o pacote da [página de Releases](https://github.com/jousudo/chatic/releases) e instale:
```bash
# Debian/Ubuntu
sudo apt install ./chatic_*_amd64.deb
# Fedora/RHEL
sudo dnf install ./chatic_*_amd64.rpm
```
O pacote cria um serviço `systemd` (endurecido), inicia automaticamente e coloca os dados em `/var/lib/chatic`. Edite `/var/lib/chatic/.env` e rode `sudo systemctl restart chatic`.

### Docker
```bash
docker run -d --name chatic -p 3030:3030 -v chatic-data:/app/storage ghcr.io/jousudo/chatic:latest
```

---

## Compilar a partir do código

### Pré-requisitos
*   **Go** instalado (versão 1.20+) — apenas para compilar a partir do código; os pacotes de release já vêm com o binário pronto.
*   **FFmpeg** — **opcional**. Necessário só para os recursos de *áudio* (receber mensagens de voz e responder em áudio). Sem ele, o bot avisa na inicialização e todo o tutor por **texto** funciona normalmente. Para ativar áudio, instale o FFmpeg e deixe-o no PATH.
*   Dispositivo celular com WhatsApp ativo para ler o QR Code de autenticação do bot.

### Execução Local
1.  (Opcional, para áudio) instale o FFmpeg:
    ```bash
    # Debian/Ubuntu:
    sudo apt-get install ffmpeg
    ```
2.  Instale as dependências do Go:
    ```bash
    go mod tidy
    ```
3.  Compile o binário:
    ```bash
    go build -o chatic ./cmd/server
    ```
4.  Execute o bot:
    ```bash
    ./chatic
    ```
5.  Abra **http://localhost:3030/admin**, crie a senha do painel (primeiro acesso) e **escaneie o QR Code na aba de Pareamento** com o WhatsApp — o QR aparece no painel, não no console.

---

## Manual de Comandos e Uso

> Todos os comandos são em inglês. Envie `/help` a qualquer momento para ver a lista completa dentro do WhatsApp.

### Comandos Gerais (para todos os usuários autorizados)
*   `/help` - Mostra a lista de comandos disponíveis.
*   `/restart` - Reinicia a configuração completa (nome, idiomas, nível, nome da professora, interesses).
*   `/language <idioma>` - Muda apenas o idioma que você está aprendendo (ex: `/language French`).
*   `/tips` - Retorna sugestões de resposta no idioma que você está aprendendo (útil após um áudio do tutor).
*   `/grammar <tópico>` - Explica uma regra de gramática com exemplos (ex: `/grammar past tense`).
*   `/word` - Ensina uma palavra ou expressão útil do dia.
*   `/vocab <tema>` - Monta uma mini-lista de vocabulário sobre um tema (ex: `/vocab travel`).
*   `/quiz` - Aplica um quiz rápido de gramática e vocabulário (gabarito no final).
*   `/fix <frase>` - Corrige explicitamente uma frase (ou `/fix` sozinho corrige sua última mensagem).
*   `/ranking` - Exibe o placar de XP acumulado por todos os participantes.
*   `/forget` - Apaga **permanentemente** todos os seus dados (confirmação com `/forget CONFIRM`).
*   `/myai <provider> <chave> [modelo]` - Usa seu próprio provedor de IA pessoal.
*   `/newgroup <nome>` - Cria um grupo de estudo.
*   `/join <código>` - Entra em um grupo de estudo.
*   `/groupai <código> <provider> <chave> [modelo]` - Define a IA compartilhada de um grupo (admins do grupo).

### Comandos de Administração (Exclusivos para números admin)
*   `/list` - Lista todos os usuários cadastrados e seus XP.
*   `/add <número> <nome>` - Adiciona um novo número à whitelist.
*   `/delete <número>` - Remove um usuário e revoga seu acesso.
*   `/recover` - Envia um token de recuperação de senha do painel para o WhatsApp do admin.

### Comandos em Grupos do WhatsApp
O bot fica em silêncio no grupo e só age quando acionado (para conter custo e evitar spam):
*   Mencione o bot, ou use `/ask <pergunta>` e `/correct <frase>` para tirar dúvidas e receber correções.
*   `/gquiz <tema>` - Cria uma **enquete-quiz nativa** no grupo (todos votam); `/greveal` mostra a resposta.
*   `/gword` - Palavra do dia para o grupo.
*   `/gchallenge <tema>` - Propõe um desafio de prática para o grupo fazer junto.
*   `/ghelp` - Mostra a lista de comandos de grupo.

> As atividades de grupo têm limite de frequência por grupo (proteção de custo). Aulas conduzidas pelo bot (`/aula`) são um recurso planejado para o futuro.

---

## Modo Multi-conta (uma casa, vários WhatsApp)

Por padrão o bot roda **uma conta compartilhada**: um número que todos mandam DM, com whitelist e um admin gerenciando os acessos.

O **modo multi-conta** (opcional) permite que **cada pessoa da casa conecte o próprio WhatsApp** e converse com o tutor **mandando mensagem para si mesma** (self-chat, com o prefixo `!`) — **sem whitelist e sem conta de gerenciamento**. Parear o próprio aparelho já é a autorização.

**Como ativar:**
1. Defina `MULTI_ACCOUNT_ENABLED=true` no `.env` e reinicie o bot.
2. No painel (`/admin`), seção **🏠 Contas da Casa**, clique em **➕ Adicionar WhatsApp**.
3. No celular do novo usuário: **WhatsApp → Aparelhos conectados → Conectar um aparelho** e escaneie o QR.
4. Pronto — a pessoa começa a conversar com o tutor mandando mensagem para si mesma começando com `!` (ex: `!hello, let's practice`).

> ⚠️ **Privacidade:** ao parear, o bot passa a ser um **dispositivo companheiro** da conta (como o WhatsApp Web) e, tecnicamente, tem acesso às conversas do aparelho. Ele **só age** no self-chat do próprio dono e ignora todo o resto (DMs de terceiros, grupos). Ainda assim, só pareie WhatsApp de pessoas que confiam na instância que você hospeda. As contas já pareadas continuam funcionando mesmo que você desligue a flag depois (ela só controla o pareamento de **novas** contas).

---

## Privacidade e Proteção de Dados (LGPD / GDPR)

Este projeto é **self-hosted**: cada pessoa roda a própria instância. Quem hospeda é o **controlador** dos dados dos usuários daquela instância — o software oferece os mecanismos para operar em conformidade com a LGPD (Brasil) e regulamentos equivalentes (GDPR etc.).

**Quais dados são tratados**
- Número de WhatsApp, nome e preferências de aprendizado (idiomas, nível, interesses, nome escolhido para a tutora) informados no onboarding.
- Histórico de conversas com o tutor (para dar contexto ao aprendizado; limitado às mensagens recentes ao montar o contexto da IA).
- Opcionalmente, uma chave de API de IA pessoal.

**Como os dados são protegidos**
- **Minimização de acesso:** só números na *whitelist* são processados; qualquer outro remetente é descartado antes de qualquer processamento.
- **Criptografia em repouso (AES‑256‑GCM):** as chaves de API (sistema, por‑usuário e por‑grupo) **e o conteúdo das conversas** são cifrados no banco com uma chave‑mestra local (`storage/.masterkey`, permissão `0600`, mantida em arquivo separado do `.db`).
- **Sem registro de conteúdo:** o console registra apenas metadados (ex.: ID do usuário), nunca o texto das mensagens.
- **Conteúdo externo é não confiável:** textos de links e documentos são tratados como material de referência, nunca como instruções ao modelo (defesa contra *prompt injection*).

**Direitos dos titulares**
- **Direito ao esquecimento (Art. 18):** o usuário pode enviar **`/forget`** (com confirmação `/forget CONFIRM`) para apagar *permanentemente* todos os seus dados — perfil, preferências, histórico e chave pessoal. O administrador também pode apagar por usuário via `/delete <número>` ou pelo painel; ambos fazem *hard delete* (não soft‑delete).
- **Acesso/portabilidade:** os dados residem em um único arquivo SQLite (`storage/tutor.db`) sob controle de quem hospeda.

**Recomendações para quem hospeda**
- Faça backup do `storage/.masterkey` **separado** do `.db` (sem a chave, o conteúdo cifrado é irrecuperável; com os dois juntos, a cifra em repouso perde o efeito contra furto do backup).
- Considere cifra de disco do SO (BitLocker/LUKS) para defesa em profundidade.
- Informe seus usuários sobre quais dados são tratados e por quê (transparência — Art. 9).

> ⚠️ Aviso: este documento descreve os recursos técnicos disponíveis e **não constitui aconselhamento jurídico**. A conformidade final depende de como você opera a sua instância.

---

## Licença

Este projeto está licenciado sob a **Licença Apache 2.0** — consulte os arquivos `LICENSE` e `NOTICE` para obter mais detalhes.
