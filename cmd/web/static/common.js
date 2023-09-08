// Copyright (C) 2023 Haiko Schol
// SPDX-License-Identifier: GPL-3.0-or-later

// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.

// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.

// You should have received a copy of the GNU General Public License
// along with this program. If not, see <https://www.gnu.org/licenses/>.

class FavoriteStore {
    static storageKey = 'favorites';
    static iconNonFavorite = 'static/favorite.svg';
    static iconFavorite = 'static/favorite-filled.svg';

    favSet;

    constructor() {
        const favoritesItem = localStorage.getItem(FavoriteStore.storageKey);
        const favorites = JSON.parse(favoritesItem ? favoritesItem : '[]');
        this.favSet = new Set(favorites);
    }

    get size() {
        return this.favSet.size;
    }

    has(imageId) {
        return this.favSet.has(imageId);
    }

    remove(imageId) {
        this.favSet.delete(imageId);
    }

    toggle(imageId) {
        if (this.favSet.has(imageId)) {
            this.favSet.delete(imageId);
        } else {
            this.favSet.add(imageId);
        }
    }

    iconForStatus(imageId) {
        return this.favSet.has(imageId) ? FavoriteStore.iconFavorite : FavoriteStore.iconNonFavorite;
    }

    toArray() {
        return [...this.favSet];
    }

    write() {
        localStorage.setItem(FavoriteStore.storageKey, JSON.stringify(this.toArray()));
    }
}
