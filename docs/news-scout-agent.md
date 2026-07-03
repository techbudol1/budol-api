# News Scout Agent

BudolPH's News Scout is a deterministic, human-reviewed workflow built with
[CloudWeGo Eino](https://www.cloudwego.io/docs/eino/) and the official
[OpenAI Go SDK](https://github.com/openai/openai-go).

## Safety contract

- The agent never creates a published or public poll.
- A scheduled or manual scan can only create `NewsCandidate` records.
- An administrator must approve a candidate in the admin app.
- Approval creates a poll with `status=draft` and `visibility=internal`.
- Publishing remains a separate, explicit action in Poll board.
- Source titles, URLs, and metadata are untrusted data. They cannot supply
  agent instructions.
- Candidates require two independent whitelisted domains or one configured
  primary-source domain.
- The model receives metadata only; BudolPH does not copy full article bodies.

## Configuration

Add the OpenAI Platform key to `.env.server`:

```dotenv
OPENAI_API_KEY=
OPENAI_MODEL=gpt-5.5
NEWS_AGENT_ENABLED=true
NEWS_AGENT_INTERVAL_MINUTES=15
```

Source selection is configurable:

```dotenv
NEWS_AGENT_QUERY=Philippines OR Filipino OR Manila
NEWS_AGENT_SOURCE_DOMAINS=gmanetwork.com,abs-cbn.com,inquirer.net,rappler.com,philstar.com,bworldonline.com,pna.gov.ph,officialgazette.gov.ph,comelec.gov.ph,pagasa.dost.gov.ph,psa.gov.ph,bsp.gov.ph,pse.com.ph
NEWS_AGENT_PRIMARY_DOMAINS=officialgazette.gov.ph,comelec.gov.ph,pagasa.dost.gov.ph,psa.gov.ph,bsp.gov.ph,pse.com.ph
```

Restart the API after configuration:

```bash
docker compose up -d --build api
```

Open `http://localhost:3001/news-desk` to run a scan or review candidates.

## Workflow

1. Retrieve recent GDELT article metadata.
2. Restrict results to the configured domain allowlist.
3. Remove duplicate URLs and group similar headlines.
4. Require corroboration or a primary source.
5. Ask OpenAI for a strict-schema poll proposal.
6. Validate the model output, source URL, deadline, classification, and poll
   type in Go.
7. Deduplicate and store the candidate in Memgraph.
8. Wait for administrator approval or rejection.
