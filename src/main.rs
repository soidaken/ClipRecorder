#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod app;
mod btnwin;
mod capture;
mod overlay;
mod recorder;

fn main() {
    app::run();
}
