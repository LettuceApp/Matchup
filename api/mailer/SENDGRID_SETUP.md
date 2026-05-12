# SendGrid Sender Authentication + Deliverability Setup

This is a one-time operator runbook. Once these records are live and
SendGrid shows your domain as "Verified", Gmail will stop labeling
verification / password-reset emails as "via sendgrid.net" and DMARC
alignment will pass (which is the single biggest signal for inbox
placement vs spam).

There are three pieces, in order: SendGrid Domain Authentication
(CNAMEs), SPF, and DMARC.

## Prerequisites

- Owner-level access to the SendGrid account.
- Access to the DNS records of the domain you want emails to come from
  (e.g. `matchup.app`, `getmatchup.com` — whichever you own).
- About 15 minutes of work + up to 48 hours of DNS propagation.

## 1. SendGrid Domain Authentication (3 CNAMEs)

This is what stops Gmail's "via sendgrid.net" label.

1. SendGrid dashboard → **Settings** → **Sender Authentication** →
   **Authenticate Your Domain** → click **Get Started**.
2. Pick your DNS host from the dropdown (Namecheap / Cloudflare / etc).
   If yours isn't listed, pick **Other Host (Not Listed)**.
3. Enter your domain (the one you want to send from, e.g. `matchup.app`).
   Leave **Advanced Settings → Use automated security** ON — it gives
   SendGrid the ability to rotate DKIM keys without you needing to
   touch DNS again.
4. SendGrid generates 3 CNAME records. They look roughly like:

   | Host (Name) | Value (Target) |
   | --- | --- |
   | `em<N>.<your-domain>` | `u<id>.wl.sendgrid.net` |
   | `s1._domainkey.<your-domain>` | `s1.domainkey.u<id>.wl.sendgrid.net` |
   | `s2._domainkey.<your-domain>` | `s2.domainkey.u<id>.wl.sendgrid.net` |

   The `<N>` and `<id>` values are unique to your SendGrid account.
   Copy them exactly — including the trailing dots if your DNS host
   shows them.
5. In your DNS host's admin, add all three as **CNAME** records. TTL
   1 hour is fine.
6. Back in SendGrid, click **Verify**. SendGrid checks the records.
   First check often fails because DNS hasn't propagated; retry every
   5–15 minutes. Once all three show ✓, you're done with this step.

## 2. SPF

SPF tells receivers which mail servers are allowed to send on behalf
of your domain.

1. Check whether your domain already has an SPF record. From a
   terminal: `dig TXT <your-domain> +short`. Look for a line starting
   `v=spf1`.
2. If none exists, add one TXT record at the root of the domain:

   ```
   Host: @ (or your domain root, depending on your DNS host)
   Type: TXT
   Value: v=spf1 include:sendgrid.net ~all
   ```

3. If an SPF record already exists (e.g. for Google Workspace), do
   **NOT** add a second one. SPF allows only ONE `v=spf1` record per
   domain — multiple records cause a permerror and ALL mail fails.
   Instead, merge them. Example for a domain that already uses Google
   Workspace:

   ```
   v=spf1 include:_spf.google.com include:sendgrid.net ~all
   ```

4. `~all` (soft-fail) is the right ending while you're rolling out.
   You can tighten to `-all` (hard-fail) once you've watched traffic
   for a week and confirmed nothing legitimate gets rejected.

## 3. DMARC

DMARC ties SPF + DKIM together and tells receivers what to do with
mail that fails. **Don't start with `p=reject`** — start with
`p=quarantine` so misconfigured mail goes to spam instead of
disappearing entirely.

Add one TXT record:

```
Host: _dmarc.<your-domain>
Type: TXT
Value: v=DMARC1; p=quarantine; rua=mailto:postmaster@<your-domain>; pct=100; aspf=r; adkim=r
```

- `p=quarantine` — failing mail goes to spam.
- `rua=mailto:postmaster@<your-domain>` — daily aggregate reports get
  sent here. Set up `postmaster@` as a real inbox or alias; the reports
  are how you spot misconfigurations.
- `pct=100` — apply the policy to 100% of mail.
- `aspf=r`, `adkim=r` — relaxed alignment (matches the org domain, not
  the exact subdomain). Required so subdomains-of-yours can still
  pass without a separate DMARC record each.

After ~2 weeks of healthy DMARC reports, you can tighten to
`p=reject` if you want strict enforcement.

## 4. Code-side: point `SENDGRID_FROM` at the authenticated domain

Once SendGrid shows the domain as Verified:

```bash
# .env (local) and your production env config
SENDGRID_FROM=noreply@<your-domain>
```

The startup check in `mailer.CheckSenderConfig()` (called from
`server.Run()`) will log a warning if `SENDGRID_FROM` is on a generic
domain like `@gmail.com` or `@sendgrid.net`. No warning = you're good.

## Verification

After everything is live:

1. **DNS propagation:** `dig CNAME em1.<your-domain> +short` resolves
   to `u<id>.wl.sendgrid.net`. Same for `s1._domainkey` and
   `s2._domainkey`.
2. **SendGrid dashboard:** Sender Authentication page shows the domain
   as **Verified** with all three records ✓.
3. **Send a test email:** trigger a verification email to a Gmail
   address. Open it.
   - "via sendgrid.net" label is GONE.
   - Click the three-dot menu → **Show original**. Look for:
     - `SPF: PASS` for `<your-domain>`
     - `DKIM: PASS` for `<your-domain>`
     - `DMARC: PASS`
4. **Inbox placement:** the email lands in **Primary** (or at worst
   **Promotions**), not Spam.

## Common gotchas

- **CNAME with a trailing dot vs without.** Some DNS hosts add it
  automatically, others don't. If SendGrid's verifier complains, try
  both.
- **Existing SPF.** Adding a second `v=spf1` record breaks everything.
  Always merge.
- **CNAME at the root.** Most DNS hosts don't allow a CNAME at the
  apex (`@`). SendGrid's three records all use subdomains so this
  isn't usually an issue, but if you're trying to use a custom
  unsubscribe domain at the root, you'll hit it.
- **Cloudflare proxying.** If you use Cloudflare, set all three
  SendGrid CNAMEs to **DNS only** (gray cloud), not Proxied (orange
  cloud) — Cloudflare's proxy intercepts the resolution and SendGrid's
  verifier won't see the right value.
- **Reverse DNS / PTR.** SendGrid handles this on their side; you
  don't need to touch it.
