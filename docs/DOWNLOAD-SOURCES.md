# Download sources

SynoDL can browse one or more external film catalogs and send what you pick to
Download Station. Sources are **off by default** and entirely opt-in: a fresh
install has none, and nothing is contacted until an administrator adds one.

## What a source is

A source is one site SynoDL is authorized to browse on your behalf. You can
configure several, including two accounts on the same site. Each has its own
authorization, its own destination folders, and its own health.

In Discover, a **source selector** appears next to the sort control once you have
more than one. It offers **All sources** — the default — plus each source by
name. With one source configured the selector is hidden and nothing changes.

## Adding a source

**Settings → Download sources → Add a source.**

SynoDL never signs in to a source for you. These sites sit behind bot protection
or a human-solved captcha, so instead you sign in with a normal browser and paste
the resulting session material. The form shows exactly which pieces the chosen
source needs.

To capture them: sign in to the site in your browser, open the developer tools'
network tab, reload a page, then right-click the page request and choose
**Copy → Copy as cURL**. Paste what follows `-b` into the cookie field and what
follows `-H 'user-agent:` into the User-Agent field.

> **Paste the cookie line whole, names included** — `name=value; name=value` —
> not just a value. Some sites name their login cookie with a per-site hash, so
> the value on its own authenticates as nobody, and the site then answers as if
> you were a stranger. That looks exactly like an expired session, which makes it
> a genuinely confusing mistake to debug. Extra cookies in the line are ignored;
> only the ones the source needs are ever sent.

> **Treat this material as a password.** For some sources it *is* one: a site
> login cookie grants everything your account there can do, which is far more
> than a scoped API token. Download links generated for you also carry your
> account identifier. SynoDL stores all of it encrypted and never displays it
> again, but if you think it has been exposed, sign out at the site — that
> invalidates the cookie immediately.

SynoDL verifies the material before storing anything. If verification fails you
are told which of these it was:

| What you see | What it means |
|---|---|
| **Active** | Working. |
| **Needs signing in again** | The session expired or was never valid. Capture it again. |
| **No active subscription** | Signed in fine, but the account cannot download. Re-pasting will not help — the subscription is the problem. |
| Could not reach the source | The site is unreachable from your server. |

## When a site changes address

Sites of this kind get blocked periodically and publish an alternate address to
reach them at. Each source has an **Alternate address** field for that, pre-filled
with the mirror SynoDL currently knows about.

When the main address stops answering, SynoDL retries the same request against
the alternate one, and browsing carries on as normal. It goes back to the main
address by itself once that recovers — there is nothing to switch back. Only an
*availability* failure triggers this: being signed out is not an outage, and
retrying elsewhere would fail the same way while hiding the real cause.

If the site publishes a new alternate address, edit the field; it takes effect
immediately and needs no SynoDL update.

> Your saved sign-in for that source is sent to the alternate address too, so
> only enter one the site itself published. This is the single outbound address
> SynoDL will use that it did not ship with, which is why it must be `https` and
> applies to that one source only.

## Everyday behaviour

**Combined browsing.** With "All sources" selected, results are drawn from every
healthy source in turn, so each one is represented from the first screenful.
Ordering is exact within a source and approximate across them — pick a single
source from the selector when you want its exact order.

**The same film twice.** Sources carry overlapping catalogs, so a popular film
will appear once per source, each labelled with where it came from. That is
deliberate: they are different releases with different sizes, encodes and
subtitle or dubbing options, and merging them would hide one of them.

**One source having trouble.** The rest keep working. A short note above the
results names the source that dropped out and why; nothing blocks, and nothing
disappears from the sources that are healthy.

**Filters.** In combined mode the filter sheet offers only the filters every
source understands, so anything you apply really does apply to everything on
screen. Select a single source to get that source's own extra filters; switching
back drops any that the others cannot honour, and tells you it did.

## Keeping a source working

Sessions expire — some in days, some in weeks. When one does, the source shows
**Needs signing in again** and an administrator re-captures it: open the source,
paste the new value, save. Leave any field blank to keep what is already stored,
so you only re-enter what actually changed.

## Removing a source

Removing it deletes its stored session material along with it. Anyone currently
browsing that source is returned to "All sources". Downloads already sent to the
NAS are unaffected — they belong to Download Station now.

## Limits

The maximum download size applies across every source and is set once, on the
same screen.

---

SynoDL is not affiliated with any source site, and adding one is your decision as
the operator. Make sure your use of a site is consistent with its terms.
