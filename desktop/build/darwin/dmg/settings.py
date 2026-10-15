# dmgbuild settings for caveira.dmg: the window a person sees on opening
# it, with the app on the left, Applications on the right and the
# background (background.html, rendered to background.png and @2x) saying
# to drag one onto the other. make dmg passes the app's path as -Dapp and
# this folder as -Ddir.
import os.path

app = defines["app"]  # noqa: F821 (dmgbuild provides defines)
here = defines["dir"]  # noqa: F821

format = "UDZO"
filesystem = "HFS+"
files = [app]
symlinks = {"Applications": "/Applications"}
# The mounted volume wears the app's icon.
icon = os.path.join(app, "Contents", "Resources", "iconfile.icns")

background = os.path.join(here, "background.png")
window_rect = ((200, 160), (660, 452))
default_view = "icon-view"
show_status_bar = False
show_tab_view = False
show_toolbar = False
show_pathbar = False
show_sidebar = False
show_icon_preview = False

# The arrow in the background runs between these two spots.
icon_size = 96
text_size = 13
label_pos = "bottom"
icon_locations = {
    os.path.basename(app): (170, 250),
    "Applications": (490, 250),
}
