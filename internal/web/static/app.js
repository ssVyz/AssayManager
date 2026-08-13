// Progressive enhancement for the app. Nothing here is required for the pages
// to work — without JavaScript every control still functions as before.

// "Select all" master checkboxes. A checkbox with class "check-all" in a table
// header toggles every (non-master) checkbox in its own <table>, and reflects
// the group's state (checked / indeterminate) as individual boxes change.
(function () {
  function itemBoxes(master) {
    var table = master.closest("table");
    if (!table) return [];
    return Array.prototype.slice.call(
      table.querySelectorAll('input[type="checkbox"]:not(.check-all)')
    );
  }

  function syncMaster(master) {
    var boxes = itemBoxes(master);
    var checked = boxes.filter(function (b) { return b.checked; }).length;
    master.checked = boxes.length > 0 && checked === boxes.length;
    master.indeterminate = checked > 0 && checked < boxes.length;
  }

  var masters = document.querySelectorAll("input.check-all");
  Array.prototype.forEach.call(masters, function (master) {
    master.addEventListener("change", function () {
      itemBoxes(master).forEach(function (b) { b.checked = master.checked; });
      master.indeterminate = false;
    });
    itemBoxes(master).forEach(function (box) {
      box.addEventListener("change", function () { syncMaster(master); });
    });
    syncMaster(master); // reflect any pre-checked state on load
  });
})();
