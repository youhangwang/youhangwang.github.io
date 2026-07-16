# Cerebrum

> OpenWolf's learning memory. Updated automatically as the AI learns from interactions.
> Do not edit manually unless correcting an error.
> Last updated: 2026-07-16

## User Preferences

<!-- How the user likes things done. Code style, tools, patterns, communication. -->

## Key Learnings

- **Project:** youhangwang.github.io
- **Toolchain:** Ruby 3.4.1 (via chruby at `/Users/johan/.rubies/ruby-3.4.1`) + Jekyll 4.2.2 + jekyll-text-theme 2.2.6. Always `source chruby.sh && chruby ruby-3.4.1` before `bundle exec`. The blog runs via `bundle exec jekyll` — a bare `gem install jekyll` into system Ruby 2.6 is the WRONG target.
- **China network:** `rubygems.org` is unreachable from this machine. The `gem sources` mirror (`https://gems.ruby-china.com`) does **NOT** apply to bundler — `bundle install` reads the source from the `Gemfile`. Fix: `bundle config set mirror.https://rubygems.org https://gems.ruby-china.com` (stored in `.bundle/config`, keeps the Gemfile clean for git).
- **Ruby 3.4 + Jekyll 4.2.2 compat (already fixed in repo):** (1) Ruby 3.4 unbundled `csv`/`base64`/`bigdecimal`/`logger`/`ostruct`/`singleton`/`benchmark` from stdlib — they're listed in the `Gemfile` so `bundle exec` can load them. (2) Ruby 3.2+ removed `Object#tainted?`, which `liquid-4.0.3` (pinned by jekyll 4.2.x) still calls — `_plugins/ruby3_compat.rb` restores `tainted?`/`taint`/`untaint` as no-ops. Do not remove either.
- **Build verification:** `bundle exec jekyll build` → post at `_site/YYYY/MM/DD/<title>.html` (permalink is `date` = `/:year/:month/:day/:title.html`, a file not a directory).

## Do-Not-Repeat

<!-- Mistakes made and corrected. Each entry prevents the same mistake recurring. -->
<!-- Format: [YYYY-MM-DD] Description of what went wrong and what to do instead. -->

- [2026-07-16] Do **NOT** run two `bundle install` processes concurrently on the same repo — they deadlock on the bundler lock and both stall forever. Kill all stuck `bundle install` PIDs before starting a fresh one.
- [2026-07-16] `gem sources` mirror switch alone does not unblock `bundle install` — bundler ignores it. Always also set `bundle config set mirror.https://rubygems.org https://gems.ruby-china.com`.
- [2026-07-16] `bundle exec jekyll` failing with `cannot load such file -- csv` while plain `ruby -e "require 'csv'"` works = the gem is hidden by Bundler.setup (not in Gemfile). Add the gem to the `Gemfile`; a global `gem install` won't help under `bundle exec`.

## Decision Log

<!-- Significant technical decisions with rationale. Why X was chosen over Y. -->

- [2026-07-16] **Ruby 3.4 compat via Gemfile additions + _plugins shim, NOT a Jekyll 4.3 upgrade.** Jekyll 4.2.2 + liquid 4.0.3 is incompatible with Ruby 3.2+ (removed `tainted?`) and Ruby 3.4 (unbundled stdlib). Chose the minimal patch (add un-bundled gems to Gemfile + `ruby3_compat.rb` no-op shim) over bumping jekyll to 4.3, to avoid cascading theme/jekyll-text-theme breakage. Trade-off: keeps the EOL jekyll 4.2 line; revisit if upgrading the theme.
