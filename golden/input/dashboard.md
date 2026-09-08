---
title: Issue Dashboard
status: done
published: 2026-04-01
---

# Issue Dashboard

<div class="dashboard-widget" data-view="cards" data-filter="status:open" data-limit="15"></div>

New issues are created through a form wrapped in a `{=html}` raw block:

```{=html}
<form class="dashboard-new">
  <input name="name" type="text" placeholder="issue-name" required/>
</form>
```
