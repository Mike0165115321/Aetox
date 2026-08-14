---
name: search-craft
description: How to get real coverage out of a search engine that returns a handful of results per query, and when to reach for the crawler or the browser instead. Use at the start of any search that has to be thorough rather than quick.
---

# Getting coverage out of a search box

## The constraint that shapes everything

`web_search` returns about eight results per query, from one engine. That number
is the reason a thorough search is *many differently-worded queries*, not one
query read carefully. Asking the same question again in the same words returns
the same eight things.

So the unit of work is the **angle**, not the query. Before searching, write down
the angles; then run them.

## The angles that are usually worth taking

For anything that exists in a market, these find different documents:

- **Its own name.** Finds the vendor's pages — authoritative about claims, and
  the place to get the actual feature list and pricing.
- **The problem, not the product.** "how do teams do X" finds the field rather
  than the winner, and it is how you discover the competitor nobody named.
- **The name beside a rival's.** "X vs Y" finds comparisons; treat them as
  advertisements until proven otherwise, but they name the axes people compare on.
- **The complaint.** What somebody types when it went wrong — "X slow", "X
  alternative", "moved off X". This is where the truth about a product lives, and
  it is the angle most often skipped.
- **The recent.** Add the year, or search for the changelog or release notes. A
  market's shape from two years ago is a different market.

## Narrow the engine rather than filtering by eye

`allowed_domains` gets the authoritative answer when you already know where it
lives — the vendor's own docs, a standards body, a specific forum. `blocked_domains`
is for the content farm that keeps outranking the real source; block it once and
the remaining results are worth reading.

Reaching for these early is usually better than reading eight bad results
carefully.

## When search is the wrong tool

- **The site is one you already know.** Fetch it directly. Searching for a page
  you can name is a slower way to get the same page.
- **You need the whole of something.** A pricing page, a docs section, a
  changelog — crawl or map it rather than searching for each piece. The deep
  research tools exist for exactly the case where the answer is spread over many
  pages of one place.
- **The page does not render without JavaScript, or blocks fetching.** Most
  community sites are one or both. That is what the browser is for — open it and
  read it like a person. A fetch that returned a consent wall or a login page is
  not a finding about the topic.

## Knowing when to stop

Stop when new angles stop returning new sources — when the fifth query surfaces
the same three domains the first four did. That is coverage, and it is a
different signal from "I have enough to write something", which arrives much
earlier and is usually wrong.

If two angles that *should* disagree return the same claim traced to the same
origin, you have one source, not two. Say so.
