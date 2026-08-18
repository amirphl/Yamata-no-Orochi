from __future__ import annotations

from collections import OrderedDict
from typing import Any


class LinkCache:
    """A bounded per-worker LRU cache. Link mappings are immutable."""

    def __init__(self, max_entries: int) -> None:
        self._max_entries = max_entries
        self._items: OrderedDict[str, dict[str, Any]] = OrderedDict()

    def get(self, code: str) -> dict[str, Any] | None:
        item = self._items.get(code)
        if item is not None:
            self._items.move_to_end(code)
        return item

    def put(self, item: dict[str, Any]) -> None:
        code = item["code"]
        self._items[code] = item
        self._items.move_to_end(code)
        while len(self._items) > self._max_entries:
            self._items.popitem(last=False)

    def __len__(self) -> int:
        return len(self._items)

