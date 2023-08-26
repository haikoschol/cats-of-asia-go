const defaultZoomLevel = 15;
const defaultRadius = 12;
let [images, circles, map] = [[], [], null];

function makePopupContent(img) {
    const {id, timestamp} = img;
    const date = new Date(timestamp).toDateString();
    const location = formatLocation(img);

    return `<a href="images/${id}"><img src="images/${id}" alt="a photo of one or more cats"/></a>
        Taken on ${date} in ${location}`;
}

function formatLocation(img) {
    const {city, country} = img;
    return city ? `${city}, ${country}` : country
}

function addCircle(img, map, radius) {
    const circle = L.circle([img.latitude, img.longitude], {color: 'red', radius: radius});
    circle.bindPopup(makePopupContent(img));
    circle.addTo(map);
    return circle;
}

function calculateRadius(zoomLevel) {
    let radius = defaultRadius;
    if (zoomLevel >= 17) {
        radius -= (zoomLevel % 16) * 2;
    }
    return Math.max(radius, 1);
}

function updateCircleRadii(circles, zoomLevel) {
    const radius = calculateRadius(zoomLevel);
    circles.forEach(c => c.setRadius(radius));
}

async function init(divId, accessToken, startLatitude, startLongitude) {
    const {latitude, longitude, zoomLevel} = readStorage(startLatitude, startLongitude)
    map = L.map(divId).setView([latitude, longitude], zoomLevel);

    L.tileLayer(`https://api.mapbox.com/styles/v1/{id}/tiles/{z}/{x}/{y}?access_token=${accessToken}`, {
        maxZoom: 22,
        id: 'mapbox/streets-v11',
        tileSize: 512,
        zoomOffset: -1
    }).addTo(map);

    const response = await fetch('/images/');
    images = await response.json();

    const radius = calculateRadius(map.getZoom());
    circles = images.map(img => addCircle(img, map, radius));

    map.on('zoomend', () => {
        updateCircleRadii(circles, map.getZoom());
        updateStorage();
    });

    map.on('moveend', () => updateStorage());
    initPlaces(images, map);
}

function initPlaces(images, map) {
    const placesUl = document.getElementById('placesUl');
    const places = {};

    images.forEach(img => places[formatLocation(img)] = img);

    for (const label in places) {
        const li = document.createElement('li');
        const a = document.createElement('a');
        const {latitude, longitude} = places[label];

        a.innerText = label;
        a.onclick = () => {
            document.getElementById('placesDropdown').removeAttribute('open');
            map.setView([latitude, longitude], defaultZoomLevel);
            updateStorage();
        }

        li.appendChild(a);
        placesUl.appendChild(li);
    }
}

function readStorage(startLatitude, startLongitude) {
    const lsLat = localStorage.getItem('latitude')
    const lsLng = localStorage.getItem('longitude')
    const lsZoom = localStorage.getItem('zoomLevel');

    return {
        'latitude': lsLat ? Number(lsLat) : startLatitude,
        'longitude': lsLng ? Number(lsLng) : startLongitude,
        'zoomLevel': lsZoom ? Number(lsZoom) : defaultZoomLevel,
    };
}

function updateStorage() {
    const {lat, lng} = map.getCenter();

    localStorage.setItem('latitude', lat);
    localStorage.setItem('longitude', lng);
    localStorage.setItem('zoomLevel', map.getZoom());
}
