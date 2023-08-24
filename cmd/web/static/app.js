const defaultRadius = 12;
let [images, circles, map] = [[], [], null];

function makePopupContent(img) {
    const {id, city, country, timestamp} = img;
    const date = new Date(timestamp).toDateString();
    const location = city ? `${city}, ${country}` : country;

    return `<a href="images/${id}"><img src="images/${id}" alt="a photo of one or more cats"/></a>
        Taken on ${date} in ${location}`;
}

function addCircle(img, map) {
    const circle = L.circle([img.latitude, img.longitude], {color: 'red', radius: defaultRadius});
    circle.bindPopup(makePopupContent(img));
    circle.addTo(map);
    return circle;
}

function updateCircleRadii(circles, zoomLevel) {
    let radius = defaultRadius;
    if (zoomLevel >= 17) {
        radius -= (zoomLevel % 16) * 2;
    }
    radius = Math.max(radius, 1);

    circles.forEach(c => c.setRadius(radius));
}

function init(divId, accessToken, startLatitude, startLongitude) {
    map = L.map(divId).setView([startLatitude, startLongitude], 15);

    L.tileLayer(`https://api.mapbox.com/styles/v1/{id}/tiles/{z}/{x}/{y}?access_token=${accessToken}`, {
        maxZoom: 22,
        id: 'mapbox/streets-v11',
        tileSize: 512,
        zoomOffset: -1
    }).addTo(map);

    fetch('/images/')
        .then(response => response.json()
            .then(d => {
                images = d
                circles = images.map(img => addCircle(img, map));
                map.on('zoomend', () => updateCircleRadii(circles, map.getZoom()));
            }));
}
