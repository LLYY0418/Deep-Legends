"""Read public R93 sources and retain response metadata/shapes, not account rows.

Run manually with network access. This is research evidence, not a CI assertion
that an unlinked/private upstream API cannot exist.
"""
import concurrent.futures
import hashlib
import json
import os
import pathlib
import re
import urllib.error
import urllib.parse
import urllib.request
from datetime import datetime, timezone
from html.parser import HTMLParser

OUTPUT = pathlib.Path(os.environ.get("R93_RESEARCH_OUTPUT", "/tmp/deep-legends-r93/research"))
SOURCES = {
    "arena": "https://op.gg/lol/modes/arena",
    "arena-ahri": "https://op.gg/lol/modes/arena/ahri/build",
    "mayhem": "https://op.gg/lol/modes/aram-mayhem",
    "mayhem-vex": "https://op.gg/lol/modes/aram-mayhem/vex/augments",
    "mode-navigation": "https://op.gg/lol/modes",
    "ranked-control": "https://op.gg/lol/leaderboards/champions/leesin?region=kr",
    "mayhem-help": "https://help.op.gg/hc/en-us/articles/60909599637657-How-to-Check-ARAM-Mayhem-Match-History",
    "custom-help": "https://help.op.gg/hc/en-us/articles/31089445595033-I-want-to-view-my-stats-for-custom-games",
    "arena-official": "https://www.leagueoflegends.com/en-gb/news/dev/dev-leveling-up-arena/",
    "queues": "https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/queues.json",
}


class Page(HTMLParser):
    def __init__(self):
        super().__init__()
        self.title = []
        self.in_title = False
        self.hidden_text_depth = 0
        self.links = set()
        self.scripts = set()
        self.text = []

    def handle_starttag(self, tag, attrs):
        attrs = dict(attrs)
        if tag in ("script", "style"):
            self.hidden_text_depth += 1
        if tag == "title":
            self.in_title = True
        if tag == "a" and attrs.get("href"):
            self.links.add(attrs["href"])
        if tag == "script" and attrs.get("src"):
            self.scripts.add(attrs["src"])

    def handle_endtag(self, tag):
        if tag in ("script", "style"):
            self.hidden_text_depth = max(0, self.hidden_text_depth - 1)
        if tag == "title":
            self.in_title = False

    def handle_data(self, data):
        if self.hidden_text_depth:
            return
        self.text.append(data)
        if self.in_title:
            self.title.append(data)


def probe(pair):
    name, url = pair
    record = {"id": name, "url": url, "requested_at_utc": datetime.now(timezone.utc).isoformat()}
    try:
        request = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0", "Accept-Encoding": "identity"})
        try:
            response = urllib.request.urlopen(request, timeout=30)
        except urllib.error.HTTPError as error:
            response = error
        with response:
            body = response.read(5 * 1024 * 1024 + 1)
            record.update(status=response.status, final_url=response.url,
                          content_type=response.headers.get("Content-Type", ""),
                          response_date=response.headers.get("Date", ""),
                          bytes=len(body), sha256=hashlib.sha256(body).hexdigest())
        if len(body) > 5 * 1024 * 1024:
            raise ValueError("response exceeded research limit")
        if response.status != 200:
            record["limitation"] = "HTTP failure is not evidence of unsupported mode"
        elif name == "queues":
            record["selected_queues"] = [
                {key: row.get(key) for key in ("id", "name")}
                for row in json.loads(body) if row.get("id") in (1700, 1710, 1750, 2400, 3140)
            ]
        else:
            page = Page()
            page.feed(body.decode("utf-8", errors="replace"))
            text = " ".join(" ".join(page.text).split())
            record.update(
                title=" ".join(page.title),
                mode_navigation=sorted(link for link in page.links if re.search(r"/lol/modes/[^/?]+$", link)),
                leaderboard_navigation=sorted(link for link in page.links if "/lol/leaderboards/" in link and "/champions/" not in link),
                unique_player_profile_links=sum("/lol/summoners/" in link for link in page.links),
                page_scripts=sorted(page.scripts),
                markers={
                    "ranked_diamond_experts": "games played in Diamond 2 or higher" in text,
                    "champion_trios": "champion trios" in text,
                    "augment_statistics": "Augments" in text and "Pick rate" in text,
                    "mayhem_other_history_unavailable": "You cannot view the ARAM:Mayhem match history of other users." in text,
                    "custom_stats_unavailable": "stats for custom games are not available on OP.GG" in text,
                    "official_six_teams_of_three": "three players on each team with a total of six teams" in text,
                },
            )
    except Exception as error:
        record["error"] = str(error)
        record["limitation"] = "Transport/parser failure is not evidence of unsupported mode"
    return record


if __name__ == "__main__":
    OUTPUT.mkdir(parents=True, exist_ok=True)
    with concurrent.futures.ThreadPoolExecutor(max_workers=3) as executor:
        records = list(executor.map(probe, SOURCES.items()))
    (OUTPUT / "responses.json").write_text(json.dumps(records, ensure_ascii=False, indent=2) + "\n")
    for record in records:
        print(json.dumps({key: record[key] for key in ("id", "status", "bytes", "unique_player_profile_links", "markers", "selected_queues", "error") if key in record}, ensure_ascii=False))
