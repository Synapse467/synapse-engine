# Security

## Reporting a vulnerability

Please report vulnerabilities privately, through GitHub's **Report a vulnerability** button on this repository's Security tab. Do not open a public issue for something exploitable, and do not include real keys, tokens or customer data in a report.

You will get an acknowledgement, and we will agree a disclosure date with you once the fix is ready.

## What counts

- Any way for the engine to return text that is not part of an approved item, or to return an item it should not.
- A gateway request that is answered although its license, signature, purpose, quota, revocation or replay check should have refused it.
- A way to read a capsule's contents from the gateway or MCP server without a question, or when the license does not allow it.
- A way to make an answer be delivered without the use being recorded.
- A crafted capsule, document, request or MCP message that panics, hangs or uses unbounded memory.

## What does not

- Licenses checked on the licensee's own machine are cooperative by design. Someone who holds a capsule file can ignore its license terms; use the gateway when limits must be enforced. This is documented, not a vulnerability.
- Retrieval declining a question that was phrased with different words than the capsule uses.
- Anything that needs the attacker to already hold the victim's `identity.json`.

## Handling secrets

Nothing in these repositories should contain a key, seed, token or real customer data; `.env` files and `identity.json` are git-ignored. If you find one committed, report it as a vulnerability.
