(() => {
  const table = document.querySelector(".statsTable");
  if (!table) return;

  let lastIdx = null;

  const clear = () => {
    table.querySelectorAll(".isColHover").forEach((el) => el.classList.remove("isColHover"));
  };

  const setCol = (idx) => {
    if (idx === lastIdx) return;
    lastIdx = idx;
    clear();
    // idx 0 is the sticky "Player" column; we only highlight team/total columns.
    if (idx == null || idx <= 0) return;
    for (const row of table.rows) {
      const cell = row.cells[idx];
      if (cell) cell.classList.add("isColHover");
    }
  };

  table.addEventListener("mousemove", (e) => {
    const cell = e.target?.closest?.("td,th");
    if (!cell || !table.contains(cell)) return;
    setCol(cell.cellIndex);
  });

  table.addEventListener("mouseleave", () => {
    lastIdx = null;
    clear();
  });
})();

